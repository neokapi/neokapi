package com.gokapi.bridge.model;

import com.google.gson.JsonObject;
import com.google.gson.annotations.SerializedName;

/**
 * NDJSON command message received from the Go bridge.
 */
public class CommandMessage {

    @SerializedName("command")
    private String command;

    @SerializedName("params")
    private JsonObject params;

    public String getCommand() {
        return command;
    }

    public void setCommand(String command) {
        this.command = command;
    }

    public JsonObject getParams() {
        return params;
    }

    public void setParams(JsonObject params) {
        this.params = params;
    }
}
