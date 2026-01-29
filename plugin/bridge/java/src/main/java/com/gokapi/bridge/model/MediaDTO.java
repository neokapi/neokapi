package com.gokapi.bridge.model;

import com.google.gson.annotations.SerializedName;
import java.util.Map;

/**
 * Wire representation of binary/media content (gokapi model.Media).
 */
public class MediaDTO {

    @SerializedName("id")
    private String id;

    @SerializedName("mime_type")
    private String mimeType;

    @SerializedName("data")
    private String data; // base64-encoded binary

    @SerializedName("uri")
    private String uri;

    @SerializedName("alt_text")
    private String altText;

    @SerializedName("properties")
    private Map<String, String> properties;

    public String getId() {
        return id;
    }

    public void setId(String id) {
        this.id = id;
    }

    public String getMimeType() {
        return mimeType;
    }

    public void setMimeType(String mimeType) {
        this.mimeType = mimeType;
    }

    public String getData() {
        return data;
    }

    public void setData(String data) {
        this.data = data;
    }

    public String getUri() {
        return uri;
    }

    public void setUri(String uri) {
        this.uri = uri;
    }

    public String getAltText() {
        return altText;
    }

    public void setAltText(String altText) {
        this.altText = altText;
    }

    public Map<String, String> getProperties() {
        return properties;
    }

    public void setProperties(Map<String, String> properties) {
        this.properties = properties;
    }
}
