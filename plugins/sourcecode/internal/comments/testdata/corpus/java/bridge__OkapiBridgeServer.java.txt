package neokapi.bridge;

import neokapi.bridge.grpc.BridgeServiceImpl;
import neokapi.bridge.io.LocalContentResolver;
import neokapi.bridge.io.LocalOutputWriter;
import neokapi.bridge.model.FilterInfo;
import neokapi.bridge.util.FilterRegistry;
import neokapi.bridge.util.StepInfo;
import neokapi.bridge.util.StepRegistry;
import com.google.gson.Gson;
import com.google.gson.GsonBuilder;
import io.grpc.Server;
import io.grpc.netty.NettyServerBuilder;
import io.netty.channel.EventLoopGroup;
import io.netty.channel.ServerChannel;
import io.netty.channel.unix.DomainSocketAddress;

import java.io.IOException;
import java.net.SocketAddress;
import java.nio.file.Files;
import java.nio.file.Path;
import java.nio.file.Paths;
import java.util.List;
import java.util.Locale;
import java.util.concurrent.Executors;
import java.util.concurrent.ScheduledExecutorService;
import java.util.concurrent.TimeUnit;

/**
 * Main entry point for the Okapi Bridge gRPC server.
 * Starts a gRPC server on a random port and prints the address to stdout
 * so the Go client can connect. All logging goes to stderr.
 *
 * <p>Flags:
 * <ul>
 *   <li>{@code --list-filters} — Print filter metadata as JSON to stdout and exit.</li>
 *   <li>{@code --list-steps} — Print step metadata as JSON to stdout and exit.</li>
 *   <li>{@code --list-capabilities} — Print both filters and steps as JSON to stdout and exit.</li>
 *   <li>{@code --idle-timeout <seconds>} — Shut down after N seconds with no active streams.
 *       Default: 0 (no timeout, subprocess mode).</li>
 *   <li>{@code --concurrency <N>} — Number of filter threads. Default: available processors.</li>
 *   <li>{@code --stuck-timeout <seconds>} — Abort if translation queue poll exceeds this.
 *       Default: 120s.</li>
 * </ul>
 */
public class OkapiBridgeServer {

    public static void main(String[] args) {
        // Plugin protocol v1 subcommands (#438). When invoked as the
        // manifest-driven plugin binary, kapi calls one of:
        //   kapi-okapi-bridge daemon   — Mode-C daemon, emits canonical handshake
        //   kapi-okapi-bridge version  — print plugin version + exit
        // The legacy --list-filters / --list-steps / --list-capabilities
        // flags also still work for build-time introspection.
        boolean daemonMode = false;
        if (args.length > 0) {
            switch (args[0]) {
                case "version":
                    System.out.println(pluginVersion());
                    return;
                case "daemon":
                    daemonMode = true;
                    // shift args so subsequent flag parsing ignores "daemon"
                    args = trimFirst(args);
                    break;
                case "pseudo":
                    // Pseudo-translate one document through Okapi's
                    // TextModificationStep (TYPE_EXTREPLACE / SCRIPT_EXT_LATIN).
                    // Used by the parity round-trip harness as the comparator.
                    System.exit(PseudoCommand.run(trimFirst(args)));
                    return;
                default:
                    // fall through to legacy flag parsing
            }
        }

        long idleTimeoutSeconds = daemonMode ? 300 : 0;
        long stuckTimeoutSeconds = 120;
        int concurrency = Runtime.getRuntime().availableProcessors();

        for (int i = 0; i < args.length; i++) {
            if ("--list-filters".equals(args[i])) {
                listFiltersAndExit();
                return;
            }
            if ("--list-steps".equals(args[i])) {
                listStepsAndExit();
                return;
            }
            if ("--list-capabilities".equals(args[i])) {
                listCapabilitiesAndExit();
                return;
            }
            if ("--idle-timeout".equals(args[i]) && i + 1 < args.length) {
                idleTimeoutSeconds = parseTimeout(args[++i]);
            }
            if ("--concurrency".equals(args[i]) && i + 1 < args.length) {
                concurrency = Integer.parseInt(args[++i]);
            }
            if ("--stuck-timeout".equals(args[i]) && i + 1 < args.length) {
                stuckTimeoutSeconds = parseTimeout(args[++i]);
            }
        }

        System.err.println("[bridge] Okapi Bridge Server starting (gRPC)...");
        System.err.println("[bridge] Concurrency: " + concurrency + " filter threads");

        try {
            BridgeServiceImpl service = new BridgeServiceImpl(
                    new LocalContentResolver(), new LocalOutputWriter(),
                    concurrency, idleTimeoutSeconds, stuckTimeoutSeconds);

            // Resolve the socket address. In daemon mode (kapi plugin
            // protocol v1), self-allocate a per-PID socket if the env var
            // is unset. Otherwise fall back to the legacy env var or TCP.
            String socketPath = System.getenv("NEOKAPI_BRIDGE_SOCKET");
            if ((socketPath == null || socketPath.isEmpty()) && daemonMode) {
                socketPath = defaultDaemonSocketPath();
            }
            Server server;
            String address;
            String handshakeSocket;

            if (socketPath != null && !socketPath.isEmpty()) {
                server = createUnixSocketServer(service, socketPath);
                address = socketPath;
                handshakeSocket = socketPath;
            } else {
                server = createTcpServer(service);
                int port = server.getPort();
                address = "localhost:" + port;
                handshakeSocket = "tcp://" + address;
            }

            System.err.println("[bridge] gRPC server started on " + address);
            if (idleTimeoutSeconds > 0) {
                System.err.println("[bridge] Idle timeout: " + idleTimeoutSeconds + "s");
            }
            if (stuckTimeoutSeconds != 120) {
                System.err.println("[bridge] Stuck timeout: " + stuckTimeoutSeconds + "s");
            }

            if (daemonMode) {
                // Plugin protocol v1 canonical handshake JSON.
                String escaped = handshakeSocket.replace("\\", "\\\\").replace("\"", "\\\"");
                System.out.println("{\"socket\":\"" + escaped + "\",\"version\":\"" + pluginVersion() + "\"}");
            } else {
                // Legacy mode: bare address line for the Go shim and tests.
                System.out.println(address);
            }
            System.out.flush();

            // Start parent process heartbeat (deadman's switch).
            // Only in subprocess mode — in daemon mode, the JVM is meant to
            // survive parent exits and rely on idle timeout instead.
            if (idleTimeoutSeconds == 0) {
                startParentHeartbeat();
            }

            // Wait for shutdown signal from the client or idle timeout.
            service.awaitShutdown();

            System.err.println("[bridge] Shutting down gRPC server...");
            server.shutdown();
            if (!server.awaitTermination(5, TimeUnit.SECONDS)) {
                server.shutdownNow();
            }

            System.err.println("[bridge] Server stopped");
        } catch (Exception e) {
            System.err.println("[bridge] Fatal error: " + e.getMessage());
            e.printStackTrace(System.err);
            System.exit(1);
        }
    }

    /**
     * Plugin version, read from the JAR's MANIFEST.MF Implementation-Version.
     * jpackage's runtime image preserves this. Falls back to "0.0.0-dev" when
     * the JAR is run from an IDE / unshaded build.
     */
    private static String pluginVersion() {
        String v = OkapiBridgeServer.class.getPackage().getImplementationVersion();
        return (v == null || v.isEmpty()) ? "0.0.0-dev" : v;
    }

    /**
     * Generate a per-PID socket path under $TMPDIR (or %TEMP% on Windows).
     * Used when running in daemon mode without a NEOKAPI_BRIDGE_SOCKET override.
     */
    private static String defaultDaemonSocketPath() {
        String tmp = System.getProperty("java.io.tmpdir");
        long pid = ProcessHandle.current().pid();
        Path dir = Paths.get(tmp);
        try {
            Files.createDirectories(dir);
        } catch (IOException ignored) {
            // tmp dir always exists on every platform we ship to; the
            // create call here just no-ops in practice.
        }
        return dir.resolve("kapi-okapi-bridge-" + pid + ".sock").toString();
    }

    /**
     * Returns args[1..n] (drops the leading subcommand). Pure-Java equivalent
     * of slicing args after we consume the daemon/version verb.
     */
    private static String[] trimFirst(String[] args) {
        if (args.length <= 1) {
            return new String[0];
        }
        String[] out = new String[args.length - 1];
        System.arraycopy(args, 1, out, 0, out.length);
        return out;
    }

    /**
     * Start a watchdog thread that checks if the parent process is still alive.
     * If the parent dies (e.g., kapi crashes or is killed), the JVM exits within 5 seconds.
     * This is the cross-platform equivalent of Linux's PDEATHSIG.
     */
    private static void startParentHeartbeat() {
        ProcessHandle.current().parent().ifPresent(parent -> {
            long parentPid = parent.pid();
            ScheduledExecutorService watchdog = Executors.newSingleThreadScheduledExecutor(r -> {
                Thread t = new Thread(r, "parent-heartbeat");
                t.setDaemon(true);
                return t;
            });
            watchdog.scheduleAtFixedRate(() -> {
                if (!ProcessHandle.of(parentPid).isPresent()) {
                    System.err.println("[bridge] Parent process " + parentPid + " gone, shutting down");
                    System.exit(0);
                }
            }, 5, 5, TimeUnit.SECONDS);
        });
    }

    /**
     * Parse a timeout value. Accepts plain seconds ("30") or duration suffix ("30s").
     */
    private static long parseTimeout(String value) {
        value = value.trim();
        if (value.endsWith("s")) {
            value = value.substring(0, value.length() - 1);
        }
        try {
            return Long.parseLong(value);
        } catch (NumberFormatException e) {
            System.err.println("[bridge] Invalid timeout value: " + value);
            return 0;
        }
    }

    /**
     * Create a gRPC server listening on a Unix domain socket.
     * Uses Netty 4.1.110+ NioServerDomainSocketChannel which leverages
     * Java 16+ NIO UnixDomainSocketAddress — no native kqueue/epoll needed.
     * Works on Linux, macOS, and Windows 10+.
     */
    @SuppressWarnings("unchecked")
    private static Server createUnixSocketServer(BridgeServiceImpl service, String socketPath) throws Exception {
        SocketAddress address = new DomainSocketAddress(socketPath);
        String os = System.getProperty("os.name", "").toLowerCase();

        EventLoopGroup bossGroup;
        EventLoopGroup workerGroup;
        Class<? extends ServerChannel> channelType;

        // Use native kqueue (macOS) or epoll (Linux) for optimized UDS.
        // Native transports use kernel-level zero-copy and bypass JDK NIO overhead.
        if (os.contains("mac")) {
            bossGroup = createEventLoopGroup("io.netty.channel.kqueue.KQueueEventLoopGroup", 1);
            workerGroup = createEventLoopGroup("io.netty.channel.kqueue.KQueueEventLoopGroup", 0);
            channelType = loadChannelClass("io.netty.channel.kqueue.KQueueServerDomainSocketChannel");
        } else if (os.contains("linux")) {
            bossGroup = createEventLoopGroup("io.netty.channel.epoll.EpollEventLoopGroup", 1);
            workerGroup = createEventLoopGroup("io.netty.channel.epoll.EpollEventLoopGroup", 0);
            channelType = loadChannelClass("io.netty.channel.epoll.EpollServerDomainSocketChannel");
        } else {
            throw new UnsupportedOperationException("Unix sockets not supported on " + os);
        }

        System.err.println("[bridge] Using Unix domain socket (" + channelType.getSimpleName() + "): " + socketPath);
        return NettyServerBuilder.forAddress(address)
                .channelType(channelType)
                .bossEventLoopGroup(bossGroup)
                .workerEventLoopGroup(workerGroup)
                .addService(service)
                .maxInboundMessageSize(64 * 1024 * 1024)
                .flowControlWindow(4 * 1024 * 1024)
                .initialFlowControlWindow(4 * 1024 * 1024)
                .build()
                .start();
    }

    /**
     * Create a gRPC server on a random TCP port (default / fallback).
     */
    private static Server createTcpServer(BridgeServiceImpl service) throws Exception {
        return NettyServerBuilder.forPort(0)
                .addService(service)
                .maxInboundMessageSize(64 * 1024 * 1024)
                .flowControlWindow(4 * 1024 * 1024)
                .initialFlowControlWindow(4 * 1024 * 1024)
                .build()
                .start();
    }

    @SuppressWarnings("unchecked")
    private static EventLoopGroup createEventLoopGroup(String className, int nThreads) throws Exception {
        return (EventLoopGroup) Class.forName(className).getConstructor(int.class).newInstance(nThreads);
    }

    @SuppressWarnings("unchecked")
    private static Class<? extends ServerChannel> loadChannelClass(String className) throws Exception {
        return (Class<? extends ServerChannel>) Class.forName(className);
    }

    /**
     * Print all discovered steps as a JSON object to stdout, then exit.
     */
    private static void listStepsAndExit() {
        try {
            List<StepInfo> steps = StepRegistry.listSteps();
            Gson gson = new GsonBuilder().create();
            System.out.println("{\"steps\":" + gson.toJson(steps) + "}");
        } catch (Exception e) {
            System.err.println("[bridge] Error listing steps: " + e.getMessage());
            System.exit(1);
        }
    }

    /**
     * Print both filters and steps as a JSON object to stdout, then exit.
     */
    private static void listCapabilitiesAndExit() {
        try {
            Gson gson = new GsonBuilder().create();
            List<FilterInfo> filters = FilterRegistry.listFilters();
            List<StepInfo> steps = StepRegistry.listSteps();
            System.out.println("{\"filters\":" + gson.toJson(filters) + ",\"steps\":" + gson.toJson(steps) + "}");
        } catch (Exception e) {
            System.err.println("[bridge] Error listing capabilities: " + e.getMessage());
            System.exit(1);
        }
    }

    /**
     * Print all discovered filters as a JSON object to stdout, then exit.
     */
    private static void listFiltersAndExit() {
        try {
            List<FilterInfo> filters = FilterRegistry.listFilters();
            Gson gson = new GsonBuilder().create();
            System.out.println("{\"filters\":" + gson.toJson(filters) + "}");
        } catch (Exception e) {
            System.err.println("[bridge] Error listing filters: " + e.getMessage());
            System.exit(1);
        }
    }
}
