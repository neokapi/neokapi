package com.gokapi.bridge.model;

import com.google.gson.JsonElement;
import com.google.gson.annotations.SerializedName;

/**
 * NDJSON response message sent to the Go bridge.
 */
public class ResponseMessage {

    @SerializedName("status")
    private String status;

    @SerializedName("data")
    private JsonElement data;

    @SerializedName("error")
    private String error;

    public static ResponseMessage ok(JsonElement data) {
        ResponseMessage msg = new ResponseMessage();
        msg.status = "ok";
        msg.data = data;
        return msg;
    }

    public static ResponseMessage ok() {
        ResponseMessage msg = new ResponseMessage();
        msg.status = "ok";
        return msg;
    }

    public static ResponseMessage error(String error) {
        ResponseMessage msg = new ResponseMessage();
        msg.status = "error";
        msg.error = error;
        return msg;
    }

    public String getStatus() {
        return status;
    }

    public JsonElement getData() {
        return data;
    }

    public String getError() {
        return error;
    }
}
