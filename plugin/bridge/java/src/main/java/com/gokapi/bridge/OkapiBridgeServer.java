package com.gokapi.bridge;

import com.gokapi.bridge.model.CommandMessage;
import com.gokapi.bridge.model.ResponseMessage;
import com.google.gson.Gson;
import com.google.gson.GsonBuilder;
import com.google.gson.JsonObject;

import java.io.BufferedReader;
import java.io.InputStreamReader;
import java.io.PrintWriter;
import java.nio.charset.StandardCharsets;

/**
 * Main entry point for the Okapi Bridge server.
 * Reads NDJSON commands from stdin, dispatches them via CommandHandler,
 * and writes NDJSON responses to stdout. All logging goes to stderr.
 */
public class OkapiBridgeServer {

    private static final Gson GSON = new GsonBuilder()
            .disableHtmlEscaping()
            .create();

    public static void main(String[] args) {
        // All logging to stderr so stdout stays clean for NDJSON protocol.
        System.err.println("[bridge] Okapi Bridge Server starting...");

        PrintWriter out = new PrintWriter(System.out, false, StandardCharsets.UTF_8);
        CommandHandler handler = new CommandHandler();

        // Send ready signal.
        JsonObject readyData = new JsonObject();
        readyData.addProperty("ready", true);
        out.println(GSON.toJson(ResponseMessage.ok(readyData)));
        out.flush();

        System.err.println("[bridge] Ready signal sent, entering command loop");

        try (BufferedReader reader = new BufferedReader(
                new InputStreamReader(System.in, StandardCharsets.UTF_8))) {

            String line;
            while ((line = reader.readLine()) != null) {
                line = line.trim();
                if (line.isEmpty()) {
                    continue;
                }

                try {
                    CommandMessage cmd = GSON.fromJson(line, CommandMessage.class);
                    String command = cmd.getCommand();

                    System.err.println("[bridge] Received command: " + command);

                    if ("shutdown".equals(command)) {
                        System.err.println("[bridge] Shutting down");
                        break;
                    }

                    ResponseMessage response = handler.handle(cmd);
                    out.println(GSON.toJson(response));
                    out.flush();

                } catch (Exception e) {
                    System.err.println("[bridge] Error processing command: " + e.getMessage());
                    e.printStackTrace(System.err);
                    out.println(GSON.toJson(ResponseMessage.error(e.getMessage())));
                    out.flush();
                }
            }
        } catch (Exception e) {
            System.err.println("[bridge] Fatal error: " + e.getMessage());
            e.printStackTrace(System.err);
            System.exit(1);
        }

        System.err.println("[bridge] Server stopped");
    }
}
