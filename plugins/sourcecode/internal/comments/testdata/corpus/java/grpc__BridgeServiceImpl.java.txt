package neokapi.bridge.grpc;

import neokapi.bridge.EventConverter;
import neokapi.bridge.io.ContentResolver;
import neokapi.bridge.io.OutputWriter;
import neokapi.bridge.model.*;
import neokapi.bridge.proto.*;
import neokapi.bridge.util.FilterRegistry;
import neokapi.bridge.util.ParameterApplier;
import neokapi.bridge.util.ParameterFlattener;
import neokapi.bridge.util.SchemaValidator;
import com.google.gson.JsonElement;
import com.google.gson.JsonObject;
import com.google.gson.JsonParser;
import com.google.protobuf.ByteString;
import io.grpc.stub.StreamObserver;
import net.sf.okapi.common.Event;
import net.sf.okapi.common.IParameters;
import net.sf.okapi.common.LocaleId;
import net.sf.okapi.common.encoder.EncoderManager;
import net.sf.okapi.common.filters.IFilter;
import net.sf.okapi.common.filterwriter.GenericFilterWriter;
import net.sf.okapi.common.filterwriter.IFilterWriter;
import net.sf.okapi.common.filterwriter.ZipFilterWriter;
import net.sf.okapi.common.resource.ITextUnit;
import net.sf.okapi.common.resource.RawDocument;

import java.io.*;
import java.nio.charset.StandardCharsets;
import java.nio.file.Files;
import java.util.*;
import java.util.concurrent.*;
import java.util.concurrent.atomic.AtomicInteger;
import java.util.concurrent.atomic.AtomicLong;

/**
 * gRPC implementation of the BridgeService.
 * Implements the single bidirectional-streaming Process RPC with a batched
 * single-pass design:
 * <ul>
 *   <li>One filter read pass: events are streamed to Go as encountered, but
 *       writing is deferred until a batch of subscribed (Block) events
 *       accumulates. By the time the batch is flushed to the writer, Go has
 *       had time to process and return translations for the earlier blocks.</li>
 *   <li>The translation queue acts as a buffer between send and write within
 *       each batch, amortizing gRPC round-trip latency across BATCH_SIZE
 *       blocks.</li>
 * </ul>
 */
public class BridgeServiceImpl extends BridgeServiceGrpc.BridgeServiceImplBase {

    private final ContentResolver contentResolver;
    private final OutputWriter outputWriter;
    private final CountDownLatch shutdownLatch = new CountDownLatch(1);
    private final ExecutorService filterPool;
    private final ExecutorService writerPool;
    private final java.util.concurrent.Semaphore pipelineSemaphore;
    private final long stuckTimeoutSeconds;
    private final Map<String, ParameterFlattener> flattenerCache = new ConcurrentHashMap<>();
    private final Map<String, JsonObject> schemaCache = new ConcurrentHashMap<>();

    // Idle timeout tracking.
    private final AtomicInteger activeStreams = new AtomicInteger(0);
    private final AtomicLong lastActivityNanos = new AtomicLong(System.nanoTime());
    private final long idleTimeoutNanos;
    private ScheduledExecutorService idleTimer;

    public BridgeServiceImpl(ContentResolver contentResolver, OutputWriter outputWriter,
                             int concurrency, long idleTimeoutSeconds, long stuckTimeoutSeconds) {
        this.contentResolver = contentResolver;
        this.outputWriter = outputWriter;
        this.stuckTimeoutSeconds = stuckTimeoutSeconds;
        this.filterPool = new ThreadPoolExecutor(
                concurrency, concurrency, 60, TimeUnit.SECONDS,
                new LinkedBlockingQueue<>(),
                r -> { Thread t = new Thread(r, "filter-pool"); t.setDaemon(true); return t; }
        );
        this.writerPool = Executors.newCachedThreadPool(
                r -> { Thread t = new Thread(r, "writer-pool"); t.setDaemon(true); return t; }
        );
        this.pipelineSemaphore = new java.util.concurrent.Semaphore(concurrency);
        this.idleTimeoutNanos = TimeUnit.SECONDS.toNanos(idleTimeoutSeconds);

        if (idleTimeoutNanos > 0) {
            long checkInterval = Math.max(idleTimeoutSeconds / 2, 1);
            idleTimer = Executors.newSingleThreadScheduledExecutor(r -> {
                Thread t = new Thread(r, "idle-timer");
                t.setDaemon(true);
                return t;
            });
            idleTimer.scheduleAtFixedRate(this::checkIdle,
                    checkInterval, checkInterval, TimeUnit.SECONDS);
        }
    }

    public BridgeServiceImpl(ContentResolver contentResolver, OutputWriter outputWriter) {
        this(contentResolver, outputWriter,
                Runtime.getRuntime().availableProcessors(), 0, 120);
    }

    public BridgeServiceImpl(ContentResolver contentResolver, OutputWriter outputWriter,
                             long idleTimeoutSeconds) {
        this(contentResolver, outputWriter,
                Runtime.getRuntime().availableProcessors(), idleTimeoutSeconds, 120);
    }

    private void checkIdle() {
        if (activeStreams.get() > 0) return;
        long elapsed = System.nanoTime() - lastActivityNanos.get();
        if (elapsed >= idleTimeoutNanos) {
            System.err.println("[bridge] Idle timeout (" + TimeUnit.NANOSECONDS.toSeconds(idleTimeoutNanos) + "s), shutting down");
            shutdownLatch.countDown();
        }
    }

    private void streamStarted() {
        activeStreams.incrementAndGet();
        lastActivityNanos.set(System.nanoTime());
    }

    private void streamEnded() {
        activeStreams.decrementAndGet();
        lastActivityNanos.set(System.nanoTime());
    }

    /**
     * Block until the Shutdown RPC is called or idle timeout triggers.
     */
    public void awaitShutdown() throws InterruptedException {
        shutdownLatch.await();
    }

    // ── Process RPC ────────────────────────────────────────────────────────────

    @Override
    public StreamObserver<ProcessRequest> process(StreamObserver<ProcessResponse> responseObserver) {
        // Limit concurrent pipelines to prevent JVM overload.
        if (!pipelineSemaphore.tryAcquire()) {
            responseObserver.onError(io.grpc.Status.RESOURCE_EXHAUSTED
                    .withDescription("too many concurrent pipelines (limit: " +
                            pipelineSemaphore.availablePermits() + ")")
                    .asRuntimeException());
            return new StreamObserver<ProcessRequest>() {
                @Override public void onNext(ProcessRequest r) {}
                @Override public void onCompleted() {}
                @Override public void onError(Throwable t) {}
            };
        }
        streamStarted();

        return new StreamObserver<ProcessRequest>() {
            private ProcessHeader header;
            private final BlockingQueue<TranslationEntry> translationQueue = new LinkedBlockingQueue<>();

            @Override
            public void onNext(ProcessRequest request) {
                switch (request.getRequestCase()) {
                    case HEADER:
                        header = request.getHeader();
                        filterPool.submit(() -> runPipeline(header, translationQueue, responseObserver));
                        break;

                    case PART:
                        handleProcessedPart(request.getPart());
                        break;

                    case PART_BATCH:
                        for (PartMessage pm : request.getPartBatch().getPartsList()) {
                            handleProcessedPart(pm);
                        }
                        break;

                    case CONTENT_BLOCK:
                        handleContentBlock(request.getContentBlock());
                        break;

                    case CONTENT_BATCH:
                        for (ContentBlock cb : request.getContentBatch().getBlocksList()) {
                            handleContentBlock(cb);
                        }
                        break;

                    default:
                        break;
                }
            }

            @Override
            public void onCompleted() {
                translationQueue.offer(TranslationEntry.END);
            }

            @Override
            public void onError(Throwable t) {
                System.err.println("[bridge] Process stream error: " + t.getMessage());
                translationQueue.clear();
                translationQueue.offer(TranslationEntry.END);
            }

            private void handleProcessedPart(PartMessage pm) {
                // Use hasBlock() (proto oneof presence) rather than
                // comparing pm.getPartType() to PartDTO.TYPE_BLOCK. The
                // legacy PartDTO.TYPE_* constants disagree with the wire
                // protocol: PartDTO.TYPE_BLOCK is 4, but the proto spec
                // (neokapi_bridge.proto:376) and the Go side both use 5
                // for Block. Comparing against 4 silently dropped every
                // Block sent by neokapi's pluginhost format-writer, so
                // `kapi pseudo-translate` via bridge wrote unmodified
                // source bytes — the verification harness in pseudobench
                // surfaced this as "84/85 files succeeded but 76 produced
                // zero pseudo runes" before the fix.
                if (pm.hasBlock()) {
                    BlockMessage blockMsg = pm.getBlock();
                    String locale = header.getOutputLocale().isEmpty()
                            ? header.getTargetLocale() : header.getOutputLocale();
                    List<SegmentDTO> segments = extractTargetSegments(blockMsg, locale);
                    try {
                        translationQueue.offer(
                                new TranslationEntry(blockMsg.getId(), segments),
                                stuckTimeoutSeconds, TimeUnit.SECONDS);
                    } catch (InterruptedException e) {
                        Thread.currentThread().interrupt();
                    }
                }
            }

            private void handleContentBlock(ContentBlock cb) {
                // Must offer an entry for EVERY block (including non-translatable),
                // because the writer thread polls the queue for each TEXT_UNIT event.
                // Non-translatable blocks get null segments → writer skips them.
                List<SegmentDTO> segments = null;
                if (cb.getTranslatable()) {
                    String locale = header.getOutputLocale().isEmpty()
                            ? header.getTargetLocale() : header.getOutputLocale();
                    segments = extractContentBlockTargets(cb, locale);
                }
                try {
                    translationQueue.offer(
                            new TranslationEntry(cb.getId(), segments),
                            stuckTimeoutSeconds, TimeUnit.SECONDS);
                } catch (InterruptedException e) {
                    Thread.currentThread().interrupt();
                }
            }
        };
    }

    /** Number of content blocks to accumulate before sending a ContentBlockBatch to Go. */
    private static final int SEND_BATCH_SIZE = 1024;

    /** Capacity of the event queue between the reader and writer threads.
     *  Must be larger than SEND_BATCH_SIZE to prevent the reader from blocking
     *  on eventQueue.put() before it can send a batch to Go. */
    private static final int EVENT_QUEUE_CAPACITY = 8192;

    /** Sentinel event marking the end of the event stream. */
    private static final Event END_OF_EVENTS = new Event(net.sf.okapi.common.EventType.NO_OP);

    /**
     * Two-thread single-pass pipeline — one filter read, no double I/O.
     *
     * <p>Thread 1 (reader, this thread): reads events from the filter, sends
     * subscribed parts to Go via gRPC, and enqueues events into an event queue.
     *
     * <p>Thread 2 (writer): dequeues events, applies translations from the
     * translation queue, and writes to the filter writer.
     *
     * <p>The two threads are decoupled: the reader never blocks on translations
     * and the writer never blocks on gRPC sends. The bounded event queue provides
     * back-pressure so the reader doesn't outrun the writer by too much.
     */
    private void runPipeline(ProcessHeader header,
                             BlockingQueue<TranslationEntry> translationQueue,
                             StreamObserver<ProcessResponse> respObserver) {
        IFilter filter = null;
        IFilterWriter writer = null;
        try {
            File inputFile = resolveProcessContent(header);
            String filterClass = header.getFilterClass();
            String sourceLocale = header.getSourceLocale();
            String targetLocale = header.getTargetLocale();
            String encoding = header.getEncoding().isEmpty() ? "UTF-8" : header.getEncoding();
            String outputLocale = header.getOutputLocale().isEmpty() ? targetLocale : header.getOutputLocale();
            boolean writeEnabled = header.hasOutput() || !header.getOutputLocale().isEmpty();

            // Build subscription filter.
            Set<Integer> subscribedTypes = new HashSet<>();
            for (int pt : header.getSubscribePartsList()) {
                subscribedTypes.add(pt);
            }
            boolean sendAll = subscribedTypes.isEmpty();

            // Promote inner-XML filters to their zip-wrapping siblings when
            // the input is a container the requested filter cannot read
            // directly (e.g. okf_odf → okf_openoffice for .odt). See
            // FilterRegistry.promoteFilterForInput.
            String effectiveFilterClass = FilterRegistry.promoteFilterForInput(filterClass, inputFile);
            if (!effectiveFilterClass.equals(filterClass)) {
                System.err.println("[bridge] promoted filter " + filterClass + " → "
                        + effectiveFilterClass + " for container input " + inputFile.getName());
            }

            filter = FilterRegistry.createFilter(effectiveFilterClass);
            if (filter == null) {
                sendComplete(respObserver, "cannot instantiate filter: " + effectiveFilterClass);
                return;
            }

            // Apply a named built-in Okapi config first (e.g. okf_xml-resx),
            // then layer any explicit filter_params override on top.
            if (!header.getConfigId().isEmpty()) {
                FilterRegistry.applyNamedConfig(filter, header.getConfigId());
            }
            Map<String, String> filterParams = header.getFilterParamsMap();
            if (filterParams != null && !filterParams.isEmpty()) {
                applyFilterParams(filter, filterParams);
            }
            setupFilterConfigurationMapper(filter);

            LocaleId srcLocale = LocaleId.fromString(sourceLocale);
            LocaleId tgtLocale = LocaleId.fromString(targetLocale);
            RawDocument rawDoc = new RawDocument(inputFile.toURI(), encoding, srcLocale, tgtLocale);
            filter.open(rawDoc);

            System.err.println("[bridge] Opened filter " + filter.getClass().getName()
                    + " for " + inputFile.getName());

            // Decide pipeline mode. The default is the single-pass design
            // documented at the top of this class: one filter instance
            // serves both read and write. That fails for filters whose
            // FilterWriter holds a live reference to source-side state
            // (ZipFilterWriter dereferences ZipSkeleton.getOriginal() —
            // the source ZipFile — when copying passthrough entries) and
            // whose own filter.next() closes that state on END_DOCUMENT.
            // The asynchronous writer thread then races against the
            // reader's internal close() and trips "zip file closed"
            // (okapi-bridge#11; affects OpenOfficeFilter, ArchiveFilter).
            //
            // For those filters the right shape is two passes with two
            // independent filter instances: pass 1 reads with filter A
            // and sends parts to Go, pass 2 opens a FRESH filter B (its
            // own ZipFile lifecycle) and writes with translations from
            // the queue. The translation queue is unchanged — Go can
            // start sending translations as soon as pass 1 emits parts,
            // and pass 2's StreamingTranslationApplier blocks on it
            // exactly as the single-pass writer thread does today.
            //
            // Detection: probe createFilterWriter() and check instanceof
            // ZipFilterWriter. The probe writer is discarded in two-pass
            // mode and re-created on filter B; the constructor in
            // ZipFilterWriter has no I/O side effects so this is cheap.
            String outputPath = null;
            ByteArrayOutputStream outputStream = null;
            boolean useTwoPass = false;
            if (writeEnabled) {
                writer = filter.createFilterWriter();
                if (writer == null) {
                    throw new IllegalStateException("filter does not support writing: " + filterClass);
                }
                outputPath = resolveProcessOutputPath(header);
                if (outputPath == null) {
                    outputStream = new ByteArrayOutputStream();
                }

                useTwoPass = (writer instanceof ZipFilterWriter);
                if (useTwoPass) {
                    // Discard the probe writer — pass 2 builds a fresh
                    // writer on filter B with the same configuration.
                    try { writer.close(); } catch (Exception ignored) {}
                    writer = null;
                    System.err.println("[bridge] runPipeline two-pass mode for "
                            + effectiveFilterClass + " (writer is ZipFilterWriter, "
                            + "needs fresh filter for write to avoid shared-state race; okapi-bridge#11)");
                } else {
                    configureWriter(writer, outputPath, outputStream, LocaleId.fromString(outputLocale), encoding);
                }
            }

            int totalSent;
            if (useTwoPass) {
                // ── Pass 1: this thread reads filter A and sends parts to Go.
                // No writer involvement; events flow nowhere downstream.
                totalSent = streamPartsToGo(filter, respObserver, sendAll, subscribedTypes);
                filter.close();
                filter = null;

                // ── Pass 2: open a fresh filter B, iterate, apply translations
                // from the queue, write output. Filter B owns its own ZipFile
                // (or equivalent) so its close() can't race against in-flight
                // writer work — there's only one event flow on this thread.
                IFilter writeFilter = null;
                try {
                    writeFilter = openConfiguredFilter(header, effectiveFilterClass, inputFile,
                            encoding, srcLocale, tgtLocale);
                    writer = writeFilter.createFilterWriter();
                    if (writer == null) {
                        throw new IllegalStateException(
                                "filter does not support writing on pass 2: " + effectiveFilterClass);
                    }
                    configureWriter(writer, outputPath, outputStream, LocaleId.fromString(outputLocale), encoding);

                    StreamingTranslationApplier applier = new StreamingTranslationApplier(
                            translationQueue, LocaleId.fromString(outputLocale), stuckTimeoutSeconds);
                    while (writeFilter.hasNext()) {
                        Event ev = writeFilter.next();
                        Event modified = applier.applyTranslations(ev);
                        writer.handleEvent(modified);
                    }
                    writer.close();
                    writer = null;
                } finally {
                    if (writeFilter != null) {
                        try { writeFilter.close(); } catch (Exception ignored) {}
                    }
                }
            } else {
                // ── Single-pass: one filter instance feeds both reader and async writer.
                // ── Start writer thread ──
                BlockingQueue<Event> eventQueue = writeEnabled
                        ? new ArrayBlockingQueue<>(EVENT_QUEUE_CAPACITY) : null;
                Future<Void> writerFuture = null;
                if (writeEnabled) {
                    IFilterWriter writerRef = writer;
                    StreamingTranslationApplier applier = new StreamingTranslationApplier(
                            translationQueue, LocaleId.fromString(outputLocale), stuckTimeoutSeconds);
                    writerFuture = writerPool.submit(() -> {
                        Event ev;
                        while ((ev = eventQueue.poll(stuckTimeoutSeconds, TimeUnit.SECONDS)) != END_OF_EVENTS) {
                            if (ev == null) {
                                throw new RuntimeException("Writer thread timed out waiting for events");
                            }
                            Event modified = applier.applyTranslations(ev);
                            writerRef.handleEvent(modified);
                        }
                        return null;
                    });
                }

                // ── Reader thread (this thread): read events, send parts, enqueue ──
                List<ContentBlock> sendBatch = new ArrayList<>(SEND_BATCH_SIZE);
                totalSent = 0;

                while (filter.hasNext()) {
                    Event event = filter.next();

                    // Convert and send subscribed parts to Go BEFORE handing the
                    // event to the writer thread. EventConverter (specifically
                    // AnnotationExtractor.extractAltTranslations) iterates
                    // source.getSegments() and target.getSegments(); the writer's
                    // applyTranslations / FilterWriter.handleEvent operate on
                    // those same Container/Segments objects. If the writer thread
                    // dequeues and starts processing while we are still
                    // iterating, Okapi's Segments$1 iterator hits the now-empty
                    // backing parts list and throws IndexOutOfBoundsException
                    // (parts get reset via TextContainer.setContent/createSingleSegment
                    // when downstream code touches the same TU).
                    PartDTO partDTO = EventConverter.convert(event);
                    if (partDTO != null) {
                        boolean subscribed = sendAll || subscribedTypes.contains(partDTO.getPartType());
                        if (subscribed) {
                            sendBatch.add(toContentBlock(partDTO));
                            totalSent++;
                            if (sendBatch.size() >= SEND_BATCH_SIZE) {
                                respObserver.onNext(ProcessResponse.newBuilder()
                                        .setContentBatch(ContentBlockBatch.newBuilder()
                                                .addAllBlocks(sendBatch).build())
                                        .build());
                                sendBatch.clear();
                            }
                        }
                    }

                    // Now safe to enqueue for writer thread.
                    if (eventQueue != null) {
                        eventQueue.put(event);
                    }
                }

                // Flush remaining send batch.
                if (!sendBatch.isEmpty()) {
                    respObserver.onNext(ProcessResponse.newBuilder()
                            .setContentBatch(ContentBlockBatch.newBuilder()
                                    .addAllBlocks(sendBatch).build())
                            .build());
                }

                // Signal end of events to writer thread.
                if (eventQueue != null) {
                    eventQueue.put(END_OF_EVENTS);
                }

                // Wait for writer thread to finish.
                if (writerFuture != null) {
                    writerFuture.get();
                }

                // Close writer then filter.
                if (writer != null) { writer.close(); writer = null; }
                filter.close(); filter = null;
            }

            // Signal read done.
            respObserver.onNext(ProcessResponse.newBuilder()
                    .setReadDone(ProcessReadDone.newBuilder().build()).build());

            System.err.println("[bridge] Pipeline complete: " + totalSent + " parts ("
                    + (useTwoPass ? "two-pass" : "single-pass") + ")");

            // Build and send Complete.
            ProcessComplete.Builder complete = ProcessComplete.newBuilder();
            if (writeEnabled) {
                if (outputPath != null) {
                    complete.setOutputPath(outputPath);
                } else if (outputStream != null) {
                    complete.setOutput(ByteString.copyFrom(outputStream.toByteArray()));
                }
            }
            sendCompleteMessage(respObserver, complete.build());

        } catch (Exception e) {
            Throwable cause = e instanceof ExecutionException ? e.getCause() : e;
            String errMsg = cause.getMessage();
            if (errMsg == null || errMsg.isEmpty()) errMsg = cause.getClass().getSimpleName();
            System.err.println("[bridge] Process error: " + errMsg);
            cause.printStackTrace(System.err);
            if (writer != null) {
                try { writer.close(); } catch (Exception ignored) {}
            }
            if (filter != null) {
                try { filter.close(); } catch (Exception ignored) {}
            }
            sendComplete(respObserver, errMsg);
        }
    }

    /**
     * Configure the filter writer with locale, encoding, encoder-manager
     * reinit, and output sink. Shared by single-pass and two-pass paths
     * so both branches produce byte-identical writer state for the same
     * input. Either {@code outputPath} or {@code outputStream} is non-null
     * (mutually exclusive — see runPipeline's outputPath resolution).
     */
    private static void configureWriter(IFilterWriter writer,
                                         String outputPath,
                                         ByteArrayOutputStream outputStream,
                                         LocaleId outputLocale,
                                         String encoding) {
        writer.setOptions(outputLocale, encoding);

        // Force the writer's encoder manager to re-initialise on the
        // next updateEncoder() call in processStartDocument. During
        // filter.open() some filters (e.g. XLIFFFilter) call
        // EncoderManager.updateEncoder with the format's MIME type,
        // which caches the source file's charset encoder and line break.
        // GenericFilterWriter.processStartDocument later calls
        // updateEncoder with the same MIME type, but it short-circuits
        // (same type → early return), leaving the stale charset from
        // the source encoding (e.g. windows-1252 → entity encoding
        // of chars > U+00FF even though the output is UTF-8). Passing
        // a dummy MIME type resets the cached value so the real call
        // in processStartDocument creates a fresh encoder with the
        // correct output encoding (UTF-8) and line break.
        if (writer instanceof GenericFilterWriter) {
            EncoderManager em = ((GenericFilterWriter) writer).getEncoderManager();
            if (em != null) {
                em.updateEncoder("application/x-force-reinit");
            }
        }

        if (outputPath != null) {
            writer.setOutput(outputPath);
        } else {
            writer.setOutput(outputStream);
        }
    }

    /**
     * Create, configure, and open a fresh filter instance for the given
     * header. Used by the two-pass pipeline for filter B (pass 2). Mirrors
     * the setup done in runPipeline for filter A: same filter class
     * promotion, same parameters, same FilterConfigurationMapper, same
     * RawDocument shape. The {@code effectiveFilterClass} is precomputed
     * by the caller so both passes use the same id even if promotion's
     * inputs change between calls.
     */
    private IFilter openConfiguredFilter(ProcessHeader header,
                                          String effectiveFilterClass,
                                          File inputFile,
                                          String encoding,
                                          LocaleId srcLocale,
                                          LocaleId tgtLocale) {
        IFilter f = FilterRegistry.createFilter(effectiveFilterClass);
        if (f == null) {
            throw new IllegalStateException(
                    "cannot instantiate filter on pass 2: " + effectiveFilterClass);
        }
        // Named config first so the write pass uses the same config as the read
        // pass; explicit filter_params override on top.
        if (!header.getConfigId().isEmpty()) {
            FilterRegistry.applyNamedConfig(f, header.getConfigId());
        }
        Map<String, String> filterParams = header.getFilterParamsMap();
        if (filterParams != null && !filterParams.isEmpty()) {
            applyFilterParams(f, filterParams);
        }
        setupFilterConfigurationMapper(f);

        RawDocument rawDoc = new RawDocument(inputFile.toURI(), encoding, srcLocale, tgtLocale);
        f.open(rawDoc);
        return f;
    }

    /**
     * Iterate the read-only filter, converting each event to a PartDTO
     * and sending subscribed parts to Go as ContentBlockBatch responses.
     * Used by pass 1 of the two-pass pipeline. Events are not retained
     * — there's no writer downstream on pass 1, and any state they hold
     * is freed when the read filter closes.
     *
     * Returns the number of subscribed parts sent.
     */
    private int streamPartsToGo(IFilter readFilter,
                                 StreamObserver<ProcessResponse> respObserver,
                                 boolean sendAll,
                                 Set<Integer> subscribedTypes) {
        List<ContentBlock> sendBatch = new ArrayList<>(SEND_BATCH_SIZE);
        int totalSent = 0;
        while (readFilter.hasNext()) {
            Event event = readFilter.next();
            PartDTO partDTO = EventConverter.convert(event);
            if (partDTO == null) continue;
            boolean subscribed = sendAll || subscribedTypes.contains(partDTO.getPartType());
            if (!subscribed) continue;
            sendBatch.add(toContentBlock(partDTO));
            totalSent++;
            if (sendBatch.size() >= SEND_BATCH_SIZE) {
                respObserver.onNext(ProcessResponse.newBuilder()
                        .setContentBatch(ContentBlockBatch.newBuilder()
                                .addAllBlocks(sendBatch).build())
                        .build());
                sendBatch.clear();
            }
        }
        if (!sendBatch.isEmpty()) {
            respObserver.onNext(ProcessResponse.newBuilder()
                    .setContentBatch(ContentBlockBatch.newBuilder()
                            .addAllBlocks(sendBatch).build())
                    .build());
        }
        return totalSent;
    }

    /**
     * Two-pass write phase (legacy fallback, not called from runPipeline).
     * Opens its own filter instance and re-reads the document, applying
     * translations from the queue. Kept as a reference for cases where the
     * batched single-pass approach cannot be used (e.g., filters that don't
     * support createFilterWriter() before iteration completes).
     * Returns the ProcessComplete message (caller is responsible for sending it).
     */
    private ProcessComplete performWritePhase(ProcessHeader header,
                                               BlockingQueue<TranslationEntry> translationQueue) throws Exception {
        String outputLocale = header.getOutputLocale().isEmpty()
                ? header.getTargetLocale() : header.getOutputLocale();
        String sourceLocale = header.getSourceLocale();
        String encoding = header.getEncoding().isEmpty() ? "UTF-8" : header.getEncoding();

        File inputFile = resolveProcessContent(header);

        String effectiveFilterClass = FilterRegistry.promoteFilterForInput(
                header.getFilterClass(), inputFile);
        IFilter writeFilter = FilterRegistry.createFilter(effectiveFilterClass);
        if (writeFilter == null) {
            throw new IllegalStateException("cannot instantiate filter for write: " + effectiveFilterClass);
        }

        // Named config first so the write path matches the read path; explicit
        // filter_params override on top.
        if (!header.getConfigId().isEmpty()) {
            FilterRegistry.applyNamedConfig(writeFilter, header.getConfigId());
        }
        Map<String, String> filterParams = header.getFilterParamsMap();
        if (filterParams != null && !filterParams.isEmpty()) {
            applyFilterParams(writeFilter, filterParams);
        }
        setupFilterConfigurationMapper(writeFilter);

        IFilterWriter writer = writeFilter.createFilterWriter();
        if (writer == null) {
            throw new IllegalStateException("filter does not support writing: " + effectiveFilterClass);
        }

        String outputPath = resolveProcessOutputPath(header);
        ByteArrayOutputStream outputStream = null;
        writer.setOptions(LocaleId.fromString(outputLocale), encoding);
        if (outputPath != null) {
            writer.setOutput(outputPath);
        } else {
            outputStream = new ByteArrayOutputStream();
            writer.setOutput(outputStream);
        }

        LocaleId srcLocale = LocaleId.fromString(sourceLocale);
        LocaleId tgtLocale = LocaleId.fromString(outputLocale);
        RawDocument rawDoc = new RawDocument(inputFile.toURI(), encoding, srcLocale, tgtLocale);
        writeFilter.open(rawDoc);

        StreamingTranslationApplier applier =
                new StreamingTranslationApplier(translationQueue, LocaleId.fromString(outputLocale), stuckTimeoutSeconds);

        while (writeFilter.hasNext()) {
            Event event = writeFilter.next();
            Event modified = applier.applyTranslations(event);
            writer.handleEvent(modified);
        }

        writeFilter.close();
        writer.close();

        ProcessComplete.Builder complete = ProcessComplete.newBuilder();
        if (outputPath != null) {
            complete.setOutputPath(outputPath);
        } else if (outputStream != null) {
            complete.setOutput(ByteString.copyFrom(outputStream.toByteArray()));
        }

        System.err.println("[bridge] Process write complete");
        return complete.build();
    }

    /**
     * Send a ProcessComplete with an optional error and close the response stream.
     */
    private void sendComplete(StreamObserver<ProcessResponse> respObserver, String error) {
        ProcessComplete.Builder complete = ProcessComplete.newBuilder();
        if (error != null) {
            complete.setError(error);
        }
        sendCompleteMessage(respObserver, complete.build());
    }

    /**
     * Send a ProcessComplete message and close the response stream.
     * Synchronized on the respObserver to avoid concurrent onNext/onCompleted calls.
     *
     * <p>Releases the pipeline backpressure slot <em>before</em> the network send
     * to avoid a race: kapi receives onCompleted() and immediately recycles its
     * lane to fire the next Process call. With the semaphore sized equal to the
     * client's concurrency, that follow-up call sees zero permits and gets
     * RESOURCE_EXHAUSTED — at scale, every subsequent file in the batch fails.
     * The slot is no longer doing useful work; release it as soon as the
     * pipeline body is done. The release also lives outside the try so a
     * client-side cancellation that throws from onNext below does not leak the
     * permit (previously every cancelled stream lost one permit permanently).
     */
    private void sendCompleteMessage(StreamObserver<ProcessResponse> respObserver,
                                      ProcessComplete complete) {
        pipelineSemaphore.release();
        try {
            synchronized (respObserver) {
                respObserver.onNext(ProcessResponse.newBuilder()
                        .setComplete(complete)
                        .build());
                respObserver.onCompleted();
            }
        } catch (Exception e) {
            System.err.println("[bridge] Could not send Complete (stream closed): " + e.getMessage());
        } finally {
            streamEnded();
        }
    }

    // ── ProcessStep RPC ─────────────────────────────────────────────────────────

    @Override
    public StreamObserver<StepRequest> processStep(StreamObserver<StepResponse> responseObserver) {
        streamStarted();

        return new StreamObserver<StepRequest>() {
            private StepHeader header;
            private net.sf.okapi.common.pipeline.BasePipelineStep step;
            private final java.util.concurrent.atomic.AtomicInteger partsProcessed =
                    new java.util.concurrent.atomic.AtomicInteger(0);

            @Override
            public void onNext(StepRequest request) {
                switch (request.getRequestCase()) {
                    case HEADER:
                        header = request.getHeader();
                        try {
                            step = initializeStep(header);
                            // Send START_BATCH and START_BATCH_ITEM events.
                            // Batch events use null resources in Okapi.
                            step.handleEvent(new Event(net.sf.okapi.common.EventType.START_BATCH));
                            step.handleEvent(new Event(net.sf.okapi.common.EventType.START_BATCH_ITEM));
                        } catch (Exception e) {
                            System.err.println("[bridge] ProcessStep init error: " + e.getMessage());
                            sendStepError(responseObserver, e.getMessage());
                        }
                        break;

                    case PART:
                        if (step == null) {
                            break;
                        }
                        try {
                            processStepPart(request.getPart(), responseObserver);
                        } catch (Exception e) {
                            System.err.println("[bridge] ProcessStep part error: " + e.getMessage());
                            sendStepError(responseObserver, e.getMessage());
                        }
                        break;

                    default:
                        break;
                }
            }

            @Override
            public void onCompleted() {
                try {
                    if (step != null) {
                        // Send END_BATCH_ITEM and END_BATCH events.
                        step.handleEvent(new Event(net.sf.okapi.common.EventType.END_BATCH_ITEM));
                        step.handleEvent(new Event(net.sf.okapi.common.EventType.END_BATCH));
                        step.destroy();
                    }
                } catch (Exception e) {
                    System.err.println("[bridge] ProcessStep cleanup error: " + e.getMessage());
                }

                synchronized (responseObserver) {
                    responseObserver.onNext(StepResponse.newBuilder()
                            .setComplete(StepComplete.newBuilder()
                                    .setPartsProcessed(partsProcessed.get())
                                    .build())
                            .build());
                    responseObserver.onCompleted();
                }
                streamEnded();
            }

            @Override
            public void onError(Throwable t) {
                System.err.println("[bridge] ProcessStep stream error: " + t.getMessage());
                if (step != null) {
                    try { step.destroy(); } catch (Exception ignored) {}
                }
                streamEnded();
            }

            private void processStepPart(PartMessage pm, StreamObserver<StepResponse> respObserver) {
                // Convert PartMessage to Okapi Event.
                Event event = partMessageToEvent(pm);
                if (event == null) {
                    // Pass through unmodified for unsupported part types.
                    synchronized (respObserver) {
                        respObserver.onNext(StepResponse.newBuilder()
                                .setPart(pm)
                                .build());
                    }
                    partsProcessed.incrementAndGet();
                    return;
                }

                // Feed through the step.
                Event result = step.handleEvent(event);

                // Convert back to PartMessage.
                PartMessage outputPm;
                if (result != null) {
                    PartDTO outputDTO = EventConverter.convert(result);
                    if (outputDTO != null) {
                        outputPm = ProtoAdapter.toProto(outputDTO);
                    } else {
                        outputPm = pm; // fallback: pass through
                    }
                } else {
                    outputPm = pm; // step returned null: pass through
                }

                synchronized (respObserver) {
                    respObserver.onNext(StepResponse.newBuilder()
                            .setPart(outputPm)
                            .build());
                }
                partsProcessed.incrementAndGet();
            }
        };
    }

    /**
     * Initialize an Okapi step from the StepHeader.
     */
    private net.sf.okapi.common.pipeline.BasePipelineStep initializeStep(StepHeader header) throws Exception {
        String stepClass = header.getStepClass();
        net.sf.okapi.common.pipeline.BasePipelineStep step =
                neokapi.bridge.util.StepRegistry.createStep(stepClass);
        if (step == null) {
            throw new IllegalStateException("cannot instantiate step: " + stepClass);
        }

        // Apply step parameters.
        Map<String, String> stepParams = header.getStepParamsMap();
        if (stepParams != null && !stepParams.isEmpty()) {
            net.sf.okapi.common.IParameters params = step.getParameters();
            if (params != null) {
                com.google.gson.JsonObject jsonParams = new com.google.gson.JsonObject();
                for (Map.Entry<String, String> entry : stepParams.entrySet()) {
                    String value = entry.getValue();
                    try {
                        com.google.gson.JsonElement parsed = com.google.gson.JsonParser.parseString(value);
                        jsonParams.add(entry.getKey(), parsed);
                    } catch (Exception e) {
                        jsonParams.addProperty(entry.getKey(), value);
                    }
                }
                ParameterApplier.applyParameters(params, jsonParams);
            }
        }

        // Inject runtime parameters for source/target locale via @StepParameterMapping.
        injectStepRuntimeParams(step, header);

        System.err.println("[bridge] Initialized step " + step.getName() + " (" + stepClass + ")");
        return step;
    }

    /**
     * Inject runtime parameters (@StepParameterMapping) for locales and directories.
     * Okapi steps use annotated setter methods for runtime parameters like locales,
     * root directory, and input root directory.
     */
    private void injectStepRuntimeParams(net.sf.okapi.common.pipeline.BasePipelineStep step, StepHeader header) {
        try {
            Class<?> spmAnnotation = Class.forName("net.sf.okapi.common.pipeline.annotations.StepParameterMapping");
            Class<?> spmType = Class.forName("net.sf.okapi.common.pipeline.annotations.StepParameterType");

            // Get StepParameterType enum constants.
            Object sourceLocaleType = null;
            Object targetLocaleType = null;
            Object rootDirType = null;
            Object inputRootDirType = null;
            for (Object constant : spmType.getEnumConstants()) {
                String name = ((Enum<?>) constant).name();
                if ("SOURCE_LOCALE".equals(name)) {
                    sourceLocaleType = constant;
                } else if ("TARGET_LOCALE".equals(name)) {
                    targetLocaleType = constant;
                } else if ("ROOT_DIRECTORY".equals(name)) {
                    rootDirType = constant;
                } else if ("INPUT_ROOT_DIRECTORY".equals(name)) {
                    inputRootDirType = constant;
                }
            }

            for (java.lang.reflect.Method method : step.getClass().getMethods()) {
                java.lang.annotation.Annotation ann = method.getAnnotation((Class<? extends java.lang.annotation.Annotation>) spmAnnotation);
                if (ann == null) {
                    continue;
                }

                // Get the parameterType value from the annotation.
                java.lang.reflect.Method ptMethod = spmAnnotation.getMethod("parameterType");
                Object paramType = ptMethod.invoke(ann);

                if (paramType.equals(sourceLocaleType) && !header.getSourceLocale().isEmpty()) {
                    method.invoke(step, net.sf.okapi.common.LocaleId.fromString(header.getSourceLocale()));
                } else if (paramType.equals(targetLocaleType) && !header.getTargetLocale().isEmpty()) {
                    method.invoke(step, net.sf.okapi.common.LocaleId.fromString(header.getTargetLocale()));
                } else if (paramType.equals(rootDirType) && !header.getRootDirectory().isEmpty()) {
                    method.invoke(step, header.getRootDirectory());
                } else if (paramType.equals(inputRootDirType) && !header.getInputRootDirectory().isEmpty()) {
                    method.invoke(step, header.getInputRootDirectory());
                }
            }
        } catch (ClassNotFoundException e) {
            // StepParameterMapping annotations not available in this Okapi version.
        } catch (Exception e) {
            System.err.println("[bridge] Could not inject step runtime params: " + e.getMessage());
        }
    }

    /**
     * Convert a PartMessage to an Okapi Event for step processing.
     * Maps:
     *   Block (4) → TEXT_UNIT event
     *   Data (5) → DOCUMENT_PART event
     *   LayerStart (0) → START_DOCUMENT / START_SUBDOCUMENT
     *   LayerEnd (1) → END_DOCUMENT / END_SUBDOCUMENT
     *   GroupStart (2) → START_GROUP
     *   GroupEnd (3) → END_GROUP
     */
    private Event partMessageToEvent(PartMessage pm) {
        switch (pm.getPartType()) {
            case PartDTO.TYPE_BLOCK: {
                if (!pm.hasBlock()) return null;
                BlockMessage bm = pm.getBlock();
                // Build a TextUnit from the block message.
                ITextUnit tu = new net.sf.okapi.common.resource.TextUnit(
                        bm.getId().isEmpty() ? "tu1" : bm.getId());
                tu.setName(bm.getName());
                tu.setMimeType(bm.getMimeType());
                tu.setIsTranslatable(bm.getTranslatable());
                tu.setPreserveWhitespaces(bm.getPreserveWhitespace());
                // Set source content from segments.
                if (bm.getSourceCount() > 0) {
                    // Use the first segment's content as source text.
                    SegmentMessage firstSeg = bm.getSource(0);
                    if (firstSeg.getRunsCount() > 0) {
                        String codedText = ProtoAdapter.runsToFragment(firstSeg.getRunsList())
                                .getCodedText();
                        tu.setSourceContent(new net.sf.okapi.common.resource.TextFragment(codedText));
                    }
                }
                // Set target content for each locale.
                for (TargetEntry te : bm.getTargetsList()) {
                    LocaleId tgtLocale = LocaleId.fromString(te.getLocale());
                    net.sf.okapi.common.resource.TextContainer target = tu.createTarget(tgtLocale, false,
                            net.sf.okapi.common.IResource.CREATE_EMPTY);
                    if (te.getSegmentsCount() > 0) {
                        SegmentMessage tgtSeg = te.getSegments(0);
                        if (tgtSeg.getRunsCount() > 0) {
                            String tgtCoded = ProtoAdapter.runsToFragment(tgtSeg.getRunsList())
                                    .getCodedText();
                            target.setContent(new net.sf.okapi.common.resource.TextFragment(tgtCoded));
                        }
                    }
                }
                // Copy properties.
                for (Map.Entry<String, String> entry : bm.getPropertiesMap().entrySet()) {
                    tu.setProperty(new net.sf.okapi.common.resource.Property(
                            entry.getKey(), entry.getValue(), true));
                }
                return new Event(net.sf.okapi.common.EventType.TEXT_UNIT, tu);
            }
            case PartDTO.TYPE_DATA: {
                if (!pm.hasData()) return null;
                DataMessage dm = pm.getData();
                net.sf.okapi.common.resource.DocumentPart dp =
                        new net.sf.okapi.common.resource.DocumentPart(
                                dm.getId().isEmpty() ? "dp1" : dm.getId(), false);
                dp.setName(dm.getName());
                return new Event(net.sf.okapi.common.EventType.DOCUMENT_PART, dp);
            }
            case PartDTO.TYPE_LAYER_START: {
                if (!pm.hasLayer()) return null;
                LayerMessage lm = pm.getLayer();
                net.sf.okapi.common.resource.StartDocument sd = new net.sf.okapi.common.resource.StartDocument(
                        lm.getId().isEmpty() ? "sd1" : lm.getId());
                sd.setName(lm.getName());
                if (!lm.getLocale().isEmpty()) {
                    sd.setLocale(LocaleId.fromString(lm.getLocale()));
                }
                sd.setMultilingual(lm.getIsMultilingual());
                if (!lm.getMimeType().isEmpty()) {
                    sd.setMimeType(lm.getMimeType());
                }
                if (!lm.getEncoding().isEmpty()) {
                    sd.setEncoding(lm.getEncoding(), lm.getHasBom());
                }
                if (!lm.getLineBreak().isEmpty()) {
                    sd.setLineBreak(lm.getLineBreak());
                }
                return new Event(net.sf.okapi.common.EventType.START_DOCUMENT, sd);
            }
            case PartDTO.TYPE_LAYER_END: {
                net.sf.okapi.common.resource.Ending ending =
                        new net.sf.okapi.common.resource.Ending("ed1");
                return new Event(net.sf.okapi.common.EventType.END_DOCUMENT, ending);
            }
            case PartDTO.TYPE_GROUP_START: {
                if (!pm.hasGroupStart()) return null;
                GroupStartMessage gsm = pm.getGroupStart();
                net.sf.okapi.common.resource.StartGroup sg = new net.sf.okapi.common.resource.StartGroup(
                        null, gsm.getId().isEmpty() ? "sg1" : gsm.getId());
                sg.setName(gsm.getName());
                sg.setType(gsm.getType());
                return new Event(net.sf.okapi.common.EventType.START_GROUP, sg);
            }
            case PartDTO.TYPE_GROUP_END: {
                net.sf.okapi.common.resource.Ending ending =
                        new net.sf.okapi.common.resource.Ending("eg1");
                return new Event(net.sf.okapi.common.EventType.END_GROUP, ending);
            }
            default:
                return null;
        }
    }

    /**
     * Send a StepComplete with an error message.
     */
    private void sendStepError(StreamObserver<StepResponse> respObserver, String error) {
        synchronized (respObserver) {
            respObserver.onNext(StepResponse.newBuilder()
                    .setComplete(StepComplete.newBuilder()
                            .setPartsProcessed(0)
                            .build())
                    .build());
            respObserver.onCompleted();
        }
        streamEnded();
    }

    // ── Shutdown RPC ───────────────────────────────────────────────────────────

    @Override
    public void shutdown(ShutdownRequest request, StreamObserver<ShutdownResponse> responseObserver) {
        System.err.println("[bridge] Shutdown requested");
        filterPool.shutdownNow();
        writerPool.shutdownNow();
        responseObserver.onNext(ShutdownResponse.newBuilder().build());
        responseObserver.onCompleted();
        shutdownLatch.countDown();
    }

    // ── Content resolution helpers ─────────────────────────────────────────────

    /**
     * Resolve content for a Process request from the ContentRef in the header.
     */
    private File resolveProcessContent(ProcessHeader header) throws IOException {
        if (!header.hasInput()) {
            throw new IOException("Process requires input in header");
        }

        ContentRef ref = header.getInput();
        String extensionHint = "";

        switch (ref.getLocationCase()) {
            case PATH:
                return contentResolver.resolvePath(ref.getPath());
            case URI:
                return contentResolver.resolveUri(ref.getUri(), extensionHint);
            case INLINE:
                return contentResolver.resolveInline(ref.getInline().toByteArray(), extensionHint);
            default:
                throw new IOException("Process requires content_ref with path, uri, or inline data");
        }
    }

    /**
     * Resolve the output file path from the Process header's output ref.
     * Returns null if no output is set (output should be returned inline as bytes).
     */
    private String resolveProcessOutputPath(ProcessHeader header) throws IOException {
        if (!header.hasOutput()) return null;
        OutputRef ref = header.getOutput();
        switch (ref.getDestinationCase()) {
            case PATH:
                File parent = new File(ref.getPath()).getParentFile();
                if (parent != null) {
                    Files.createDirectories(parent.toPath());
                }
                return ref.getPath();
            case URI:
                return outputWriter.resolveUri(ref.getUri());
            default:
                return null;
        }
    }

    // ── Target extraction ──────────────────────────────────────────────────────

    /**
     * Extract target fragments for the given locale directly from a proto BlockMessage.
     * Works on proto messages without converting to DTOs, avoiding intermediate allocations.
     */
    private static List<SegmentDTO> extractTargetSegments(BlockMessage block, String locale) {
        for (TargetEntry target : block.getTargetsList()) {
            if (target.getLocale().equals(locale)) {
                List<SegmentDTO> segments = new ArrayList<>(target.getSegmentsCount());
                for (SegmentMessage seg : target.getSegmentsList()) {
                    if (seg.getRunsCount() == 0) {
                        continue;
                    }
                    SegmentDTO dto = new SegmentDTO();
                    dto.setId(seg.getId());
                    dto.setContent(ProtoAdapter.runsToFragment(seg.getRunsList()));
                    segments.add(dto);
                }
                return segments;
            }
        }
        return null;
    }

    /**
     * Extract target segments for the given locale from a ContentBlock proto.
     */
    private static List<SegmentDTO> extractContentBlockTargets(ContentBlock cb, String locale) {
        for (TargetEntry target : cb.getTargetsList()) {
            if (target.getLocale().equals(locale)) {
                List<SegmentDTO> segments = new ArrayList<>(target.getSegmentsCount());
                for (SegmentMessage seg : target.getSegmentsList()) {
                    SegmentDTO dto = new SegmentDTO();
                    dto.setId(seg.getId());
                    // Include empty-run segments so the applier still runs
                    // for blocks containing ignorables that need source-copy
                    // targets materialized (icu_message.xlf2 scenario).
                    if (seg.getRunsCount() > 0) {
                        dto.setContent(ProtoAdapter.runsToFragment(seg.getRunsList()));
                    }
                    segments.add(dto);
                }
                return segments;
            }
        }
        return null;
    }

    /**
     * Convert a PartDTO to a lightweight ContentBlock proto.
     * Carries source/target/properties/annotations/displayHint but skips
     * skeleton and is_referent for smaller wire size.
     */
    private static ContentBlock toContentBlock(PartDTO partDTO) {
        BlockDTO block = partDTO.getBlock();
        if (block == null) {
            // Non-block parts: return a minimal ContentBlock with just the ID.
            return ContentBlock.newBuilder().build();
        }

        ContentBlock.Builder cb = ContentBlock.newBuilder()
                .setId(nullSafe(block.getId()))
                .setName(nullSafe(block.getName()))
                .setType(nullSafe(block.getType()))
                .setMimeType(nullSafe(block.getMimeType()))
                .setTranslatable(block.isTranslatable())
                .setPreserveWhitespace(block.isPreserveWhitespace());

        // Source segments
        if (block.getSource() != null) {
            for (SegmentDTO seg : block.getSource()) {
                cb.addSource(ProtoAdapter.toProto(seg));
            }
        }

        // Target segments
        if (block.getTargets() != null) {
            for (TargetDTO target : block.getTargets()) {
                TargetEntry.Builder te = TargetEntry.newBuilder()
                        .setLocale(nullSafe(target.getLocale()));
                if (target.getSegments() != null) {
                    for (SegmentDTO seg : target.getSegments()) {
                        te.addSegments(ProtoAdapter.toProto(seg));
                    }
                }
                cb.addTargets(te);
            }
        }

        // Properties
        if (block.getProperties() != null) {
            cb.putAllProperties(block.getProperties());
        }

        // Annotations
        if (block.getAnnotations() != null) {
            for (Map.Entry<String, AnnotationEntryDTO> entry : block.getAnnotations().entrySet()) {
                AnnotationEntryDTO ae = entry.getValue();
                neokapi.bridge.proto.AnnotationEntry.Builder ab =
                        neokapi.bridge.proto.AnnotationEntry.newBuilder()
                                .setType(nullSafe(ae.getType()));
                if (ae.getData() != null) {
                    ab.setData(com.google.protobuf.ByteString.copyFrom(ae.getData()));
                }
                cb.putAnnotations(entry.getKey(), ab.build());
            }
        }

        // Display hint
        if (block.getDisplayHint() != null) {
            cb.setDisplayHint(ProtoAdapter.toProto(block.getDisplayHint()));
        }

        return cb.build();
    }

    // ── Filter parameter helpers ───────────────────────────────────────────────

    /** Reserved param keys handled specially by the bridge. */
    private static final java.util.Set<String> RESERVED_PARAMS = new java.util.HashSet<>(
            java.util.Arrays.asList("configFile", "fprmContent", "apiVersion", "kind", "spec",
                    neokapi.bridge.util.RegexRulesApplier.RULES_PARAM));

    /**
     * Apply filter parameters from the gRPC map&lt;string, string&gt; format.
     * Values that look like JSON are parsed back to JsonElements for
     * the ParameterApplier, which expects a JsonObject.
     *
     * <p>Special handling:
     * <ul>
     *   <li>{@code configFile} — load native Okapi config from a file path (.fprm, YAML, etc.)</li>
     *   <li>{@code fprmContent} — load native Okapi config from inline content (same formats)</li>
     * </ul>
     *
     * <p>When a schema is available for the filter, incoming JSON config is
     * validated against it (warnings logged) and hierarchical params are
     * flattened to flat Okapi names via {@link ParameterFlattener}.
     */
    private void applyFilterParams(IFilter filter, Map<String, String> originalParams) {
        // Make a mutable copy — proto maps are unmodifiable and envelope
        // unwrapping may replace the map entirely.
        Map<String, String> params = new HashMap<>(originalParams);
        IParameters filterParameters = filter.getParameters();
        if (filterParameters == null) {
            System.err.println("[bridge] Warning: Filter does not support parameters");
            return;
        }

        // Handle enveloped config: if kind is an Okf*FilterConfig, validate and unwrap.
        // The envelope format is: apiVersion (vN), kind (Okf{Format}FilterConfig),
        // spec (JSON-encoded params).
        String kind = params.get("kind");
        if (kind != null && !kind.isEmpty()) {
            // Validate kind format
            if (!kind.startsWith("Okf") || !kind.endsWith("FilterConfig")) {
                System.err.println("[bridge] Warning: unexpected kind '" + kind +
                        "' (expected Okf{Format}FilterConfig), proceeding anyway");
            }

            // If spec is present, use it as the source of filter parameters
            String specJson = params.get("spec");
            if (specJson != null && !specJson.isEmpty()) {
                try {
                    JsonElement specElement = JsonParser.parseString(specJson);
                    if (specElement.isJsonObject()) {
                        JsonObject specParams = specElement.getAsJsonObject();
                        String apiVersion = params.get("apiVersion");
                        // Replace params with the unwrapped spec (pass to the flattener + applier below)
                        params = new HashMap<>();
                        for (Map.Entry<String, JsonElement> entry : specParams.entrySet()) {
                            params.put(entry.getKey(), entry.getValue().toString());
                        }
                        System.err.println("[bridge] Unwrapped envelope config (kind=" + kind +
                                ", apiVersion=" + (apiVersion != null ? apiVersion : "unset") +
                                "), " + specParams.size() + " spec params");
                    }
                } catch (Exception e) {
                    System.err.println("[bridge] Warning: Could not parse spec from envelope: " + e.getMessage());
                }
            }
        }

        // Handle configFile: load the entire configuration from a file.
        String configFilePath = params.get("configFile");
        if (configFilePath != null && !configFilePath.isEmpty()) {
            loadConfigFromFile(filterParameters, configFilePath);
        }

        // Handle fprmContent: load native Okapi config from inline content.
        // This allows sending .fprm or YAML config inline without writing to disk.
        String fprmContent = params.get("fprmContent");
        if (fprmContent != null && !fprmContent.isEmpty()) {
            try {
                filterParameters.fromString(fprmContent);
                System.err.println("[bridge] Loaded config from inline fprmContent");
            } catch (Exception e) {
                System.err.println("[bridge] Warning: Could not parse fprmContent: " + e.getMessage());
            }
        }

        // Convert remaining params (excluding reserved keys) to JsonObject.
        // The Go side JSON-encodes non-string values (booleans, numbers, objects).
        JsonObject jsonParams = new JsonObject();
        for (Map.Entry<String, String> entry : params.entrySet()) {
            if (RESERVED_PARAMS.contains(entry.getKey())) {
                continue;
            }
            String value = entry.getValue();
            try {
                JsonElement parsed = JsonParser.parseString(value);
                jsonParams.add(entry.getKey(), parsed);
            } catch (Exception e) {
                // Not valid JSON — use as plain string.
                jsonParams.addProperty(entry.getKey(), value);
            }
        }

        if (jsonParams.size() > 0) {
            // Load schema for validation and flattening.
            JsonObject schema = loadSchema(
                    FilterRegistry.getFilterId(filter.getClass().getName()));

            // Validate against schema (warnings only — don't reject config).
            if (schema != null) {
                SchemaValidator validator = new SchemaValidator(schema);
                SchemaValidator.ValidationResult result = validator.validate(jsonParams);
                if (!result.getErrors().isEmpty()) {
                    System.err.println("[bridge] Config validation errors: " +
                            String.join("; ", result.getErrors()));
                }
                if (!result.getWarnings().isEmpty()) {
                    System.err.println("[bridge] Config validation warnings: " +
                            String.join("; ", result.getWarnings()));
                }
            }

            // Flatten hierarchical params if a schema with x-flattenPath is available.
            // This is backwards-compatible: flat input passes through unchanged.
            ParameterFlattener flattener = getFlattener(filter.getClass().getName());
            if (flattener != null) {
                jsonParams = flattener.flatten(jsonParams);
            }

            boolean success = ParameterApplier.applyParameters(filterParameters, jsonParams);
            if (success) {
                System.err.println("[bridge] Applied " + jsonParams.size() + " additional filter parameters");
            } else {
                System.err.println("[bridge] Warning: Some filter parameters could not be applied");
            }
        }

        // Handle regexRulesJson: rebuild the RegexFilter rule list from a JSON
        // array and compile it. This carries Okapi's rule-driven extraction
        // through the flat FilterParams transport (the rule list cannot be
        // expressed as flat string params; see RegexRulesApplier). Applied
        // last so any escape / regexOptions params set above are in place
        // before compileRules() runs.
        String regexRulesJson = params.get(neokapi.bridge.util.RegexRulesApplier.RULES_PARAM);
        if (regexRulesJson != null && !regexRulesJson.isEmpty()) {
            if (neokapi.bridge.util.RegexRulesApplier.supports(filterParameters)) {
                try {
                    int n = neokapi.bridge.util.RegexRulesApplier.apply(filterParameters, regexRulesJson);
                    System.err.println("[bridge] Applied " + n + " regex extraction rule(s) from regexRulesJson");
                } catch (Exception e) {
                    System.err.println("[bridge] Warning: Could not apply regexRulesJson: " + e.getMessage());
                }
            } else {
                System.err.println("[bridge] Warning: regexRulesJson supplied but filter does not support a "
                        + "RegexFilter rule list (" + filterParameters.getClass().getName() + ")");
            }
        }

        System.err.println("[bridge] Applied " + params.size() + " filter parameters");
    }

    /**
     * Load native Okapi configuration from a file path (.fprm, YAML, etc.).
     * Prefers load(URL) over fromString() for correct #v1 header handling.
     */
    private void loadConfigFromFile(IParameters filterParameters, String configFilePath) {
        try {
            java.net.URL configUrl = new File(configFilePath).toURI().toURL();
            filterParameters.load(configUrl, false);
            System.err.println("[bridge] Loaded config from file: " + configFilePath);
        } catch (Exception e) {
            try {
                String configContent = new String(Files.readAllBytes(
                        new File(configFilePath).toPath()), java.nio.charset.StandardCharsets.UTF_8);
                filterParameters.fromString(configContent);
                System.err.println("[bridge] Loaded config from file via fromString: " + configFilePath);
            } catch (Exception e2) {
                System.err.println("[bridge] Warning: Could not load config file " +
                        configFilePath + ": " + e2.getMessage());
            }
        }
    }

    // ── FilterConfigurationMapper ──────────────────────────────────────────────

    /**
     * Lazily-initialized FilterConfigurationMapper with all discovered filters
     * registered. Built once and reused across all filter instances so that
     * container/delegating filters (Archive, AutoXLIFF, MultiParsers, SdlPackage,
     * WsxzPackage) can resolve any sub-filter ID (e.g., "okf_xliff", "okf_tmx",
     * "okf_markdown") to the correct filter class.
     */
    private volatile net.sf.okapi.common.filters.FilterConfigurationMapper sharedFcMapper;

    private net.sf.okapi.common.filters.FilterConfigurationMapper getSharedFilterConfigurationMapper() {
        if (sharedFcMapper == null) {
            synchronized (this) {
                if (sharedFcMapper == null) {
                    net.sf.okapi.common.filters.FilterConfigurationMapper mapper =
                            new net.sf.okapi.common.filters.FilterConfigurationMapper();
                    for (String fc : FilterRegistry.scanFilterClassNames()) {
                        try {
                            mapper.addConfigurations(fc);
                        } catch (Exception e) {
                            // Skip filters that fail to register (e.g., missing dependencies)
                        }
                    }
                    sharedFcMapper = mapper;
                }
            }
        }
        return sharedFcMapper;
    }

    /**
     * Set up a FilterConfigurationMapper on the filter so that sub-filtering works.
     * Uses a shared mapper with all discovered filters registered, enabling
     * container filters to delegate to any sub-filter available on the classpath.
     */
    private void setupFilterConfigurationMapper(IFilter filter) {
        try {
            filter.setFilterConfigurationMapper(getSharedFilterConfigurationMapper());
        } catch (Exception e) {
            // Non-fatal: sub-filtering won't work but basic filtering will.
            System.err.println("[bridge] Warning: Could not set up FilterConfigurationMapper: " + e.getMessage());
        }
    }

    // ── Schema / Flattener caches ──────────────────────────────────────────────

    /**
     * Get or create a ParameterFlattener for the given filter class.
     * Returns null if no schema is available (flattening will be skipped).
     */
    private ParameterFlattener getFlattener(String filterClass) {
        return flattenerCache.computeIfAbsent(filterClass, fc -> {
            String filterId = FilterRegistry.getFilterId(fc);
            if (filterId == null) return null;
            JsonObject schema = loadSchema(filterId);
            if (schema == null) return null;
            return new ParameterFlattener(schema);
        });
    }

    /**
     * Load a composite JSON Schema from the classpath for the given filter ID.
     * Results are cached for the lifetime of the bridge instance.
     */
    private JsonObject loadSchema(String filterId) {
        if (filterId == null) return null;
        return schemaCache.computeIfAbsent(filterId, id -> {
            String[] paths = {
                "/schemas/" + id + ".schema.json",
                "schemas/" + id + ".schema.json"
            };
            for (String path : paths) {
                try (InputStream is = getClass().getResourceAsStream(path)) {
                    if (is != null) {
                        try (InputStreamReader reader = new InputStreamReader(is, StandardCharsets.UTF_8)) {
                            return JsonParser.parseReader(reader).getAsJsonObject();
                        }
                    }
                } catch (Exception e) {
                    System.err.println("[bridge] Could not load schema for " + id + ": " + e.getMessage());
                }
            }
            return null;
        });
    }

    private static String nullSafe(String s) {
        return s != null ? s : "";
    }
}
