package com.gokapi.bridge;

import com.gokapi.bridge.model.*;
import com.gokapi.bridge.util.FilterRegistry;
import com.google.gson.*;

import net.sf.okapi.common.Event;
import net.sf.okapi.common.LocaleId;
import net.sf.okapi.common.filters.IFilter;
import net.sf.okapi.common.filterwriter.IFilterWriter;
import net.sf.okapi.common.resource.RawDocument;

import java.io.*;
import java.nio.charset.StandardCharsets;
import java.nio.file.Files;
import java.util.ArrayList;
import java.util.Base64;
import java.util.List;

/**
 * Dispatches NDJSON commands to the appropriate handler methods.
 * Holds the current IFilter instance and manages its lifecycle.
 */
public class CommandHandler {

    private static final Gson GSON = new GsonBuilder()
            .disableHtmlEscaping()
            .create();

    private IFilter currentFilter;
    private byte[] currentContent;
    private String currentFilterClass;

    /**
     * Handle a command and return a response.
     */
    public ResponseMessage handle(CommandMessage cmd) {
        try {
            switch (cmd.getCommand()) {
                case "info":
                    return handleInfo(cmd.getParams());
                case "open":
                    return handleOpen(cmd.getParams());
                case "read":
                    return handleRead();
                case "write":
                    return handleWrite(cmd.getParams());
                case "close":
                    return handleClose();
                case "list_filters":
                    return handleListFilters();
                default:
                    return ResponseMessage.error("unknown command: " + cmd.getCommand());
            }
        } catch (Exception e) {
            System.err.println("[bridge] Command error: " + e.getMessage());
            e.printStackTrace(System.err);
            return ResponseMessage.error(e.getMessage());
        }
    }

    private ResponseMessage handleInfo(JsonObject params) throws Exception {
        String filterClass = params.get("filter_class").getAsString();
        FilterInfo info = FilterRegistry.getFilterInfo(filterClass);
        if (info == null) {
            return ResponseMessage.error("filter class not found: " + filterClass);
        }
        return ResponseMessage.ok(GSON.toJsonTree(info));
    }

    private ResponseMessage handleOpen(JsonObject params) throws Exception {
        String filterClass = params.get("filter_class").getAsString();
        String uri = params.has("uri") ? params.get("uri").getAsString() : "";
        String sourceLocale = params.has("source_locale") ? params.get("source_locale").getAsString() : "en";
        String encoding = params.has("encoding") ? params.get("encoding").getAsString() : "UTF-8";
        String contentBase64 = params.has("content_base64") ? params.get("content_base64").getAsString() : "";
        String mimeType = params.has("mime_type") ? params.get("mime_type").getAsString() : null;

        // Decode content.
        byte[] content = Base64.getDecoder().decode(contentBase64);
        currentContent = content;
        currentFilterClass = filterClass;

        // Instantiate filter.
        currentFilter = FilterRegistry.createFilter(filterClass);
        if (currentFilter == null) {
            return ResponseMessage.error("cannot instantiate filter: " + filterClass);
        }

        // Write content to a temp file so filters that need random access (e.g., OpenXML/ZIP) work.
        String ext = "";
        int dotIdx = uri.lastIndexOf('.');
        if (dotIdx >= 0) {
            ext = uri.substring(dotIdx);
        }
        File tempFile = File.createTempFile("gokapi-bridge-", ext);
        tempFile.deleteOnExit();
        Files.write(tempFile.toPath(), content);

        LocaleId srcLocale = LocaleId.fromString(sourceLocale);
        RawDocument rawDoc = new RawDocument(tempFile.toURI(), encoding, srcLocale);

        currentFilter.open(rawDoc);

        System.err.println("[bridge] Opened filter " + filterClass + " for " + uri);
        return ResponseMessage.ok();
    }

    private ResponseMessage handleRead() throws Exception {
        if (currentFilter == null) {
            return ResponseMessage.error("no filter is currently open");
        }

        List<PartDTO> parts = new ArrayList<>();
        while (currentFilter.hasNext()) {
            Event event = currentFilter.next();
            PartDTO part = EventConverter.convert(event);
            if (part != null) {
                parts.add(part);
            }
        }

        JsonObject data = new JsonObject();
        data.add("parts", GSON.toJsonTree(parts));

        System.err.println("[bridge] Read " + parts.size() + " parts");
        return ResponseMessage.ok(data);
    }

    private ResponseMessage handleWrite(JsonObject params) throws Exception {
        String filterClass = params.get("filter_class").getAsString();
        String locale = params.has("locale") ? params.get("locale").getAsString() : "";
        String encoding = params.has("encoding") ? params.get("encoding").getAsString() : "UTF-8";
        String originalBase64 = params.has("original_content_base64")
                ? params.get("original_content_base64").getAsString() : "";

        byte[] originalContent = Base64.getDecoder().decode(originalBase64);

        // Parse parts from JSON.
        JsonArray partsArray = params.getAsJsonArray("parts");
        List<PartDTO> parts = new ArrayList<>();
        for (JsonElement elem : partsArray) {
            parts.add(GSON.fromJson(elem, PartDTO.class));
        }

        // Create filter for reading the skeleton.
        IFilter filter = FilterRegistry.createFilter(filterClass);
        if (filter == null) {
            return ResponseMessage.error("cannot instantiate filter: " + filterClass);
        }

        // Create filter writer.
        IFilterWriter writer = filter.createFilterWriter();
        if (writer == null) {
            return ResponseMessage.error("filter does not support writing: " + filterClass);
        }

        ByteArrayOutputStream outputStream = new ByteArrayOutputStream();
        writer.setOptions(LocaleId.fromString(locale), encoding);
        writer.setOutput(outputStream);

        // Write original content to temp file for filters needing random access.
        File tempFile = File.createTempFile("gokapi-bridge-write-", "");
        tempFile.deleteOnExit();
        Files.write(tempFile.toPath(), originalContent);

        LocaleId srcLocale = LocaleId.fromString("en"); // source locale
        RawDocument rawDoc = new RawDocument(tempFile.toURI(), encoding, srcLocale);
        filter.open(rawDoc);

        // Feed events from the filter, replacing text units with translated content.
        PartDTOConverter converter = new PartDTOConverter(parts, LocaleId.fromString(locale));
        while (filter.hasNext()) {
            Event event = filter.next();
            Event modified = converter.applyTranslations(event);
            writer.handleEvent(modified);
        }

        filter.close();
        writer.close();

        String outputBase64 = Base64.getEncoder().encodeToString(outputStream.toByteArray());
        JsonObject data = new JsonObject();
        data.addProperty("output_base64", outputBase64);

        System.err.println("[bridge] Wrote output (" + outputStream.size() + " bytes)");
        return ResponseMessage.ok(data);
    }

    private ResponseMessage handleClose() {
        if (currentFilter != null) {
            try {
                currentFilter.close();
            } catch (Exception e) {
                System.err.println("[bridge] Error closing filter: " + e.getMessage());
            }
            currentFilter = null;
            currentContent = null;
            currentFilterClass = null;
        }
        return ResponseMessage.ok();
    }

    private ResponseMessage handleListFilters() {
        List<FilterInfo> filters = FilterRegistry.listFilters();
        JsonObject data = new JsonObject();
        data.add("filters", GSON.toJsonTree(filters));
        return ResponseMessage.ok(data);
    }
}
