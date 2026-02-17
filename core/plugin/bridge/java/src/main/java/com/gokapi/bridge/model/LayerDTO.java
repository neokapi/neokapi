package com.gokapi.bridge.model;

import com.google.gson.annotations.SerializedName;
import java.util.Map;

/**
 * Wire representation of a structural layer (gokapi model.Layer).
 */
public class LayerDTO {

    @SerializedName("id")
    private String id;

    @SerializedName("name")
    private String name;

    @SerializedName("format")
    private String format;

    @SerializedName("locale")
    private String locale;

    @SerializedName("encoding")
    private String encoding;

    @SerializedName("mime_type")
    private String mimeType;

    @SerializedName("line_break")
    private String lineBreak;

    @SerializedName("is_multilingual")
    private boolean isMultilingual;

    @SerializedName("parent_id")
    private String parentId;

    @SerializedName("properties")
    private Map<String, String> properties;

    public String getId() {
        return id;
    }

    public void setId(String id) {
        this.id = id;
    }

    public String getName() {
        return name;
    }

    public void setName(String name) {
        this.name = name;
    }

    public String getFormat() {
        return format;
    }

    public void setFormat(String format) {
        this.format = format;
    }

    public String getLocale() {
        return locale;
    }

    public void setLocale(String locale) {
        this.locale = locale;
    }

    public String getEncoding() {
        return encoding;
    }

    public void setEncoding(String encoding) {
        this.encoding = encoding;
    }

    public String getMimeType() {
        return mimeType;
    }

    public void setMimeType(String mimeType) {
        this.mimeType = mimeType;
    }

    public String getLineBreak() {
        return lineBreak;
    }

    public void setLineBreak(String lineBreak) {
        this.lineBreak = lineBreak;
    }

    public boolean isMultilingual() {
        return isMultilingual;
    }

    public void setMultilingual(boolean multilingual) {
        isMultilingual = multilingual;
    }

    public String getParentId() {
        return parentId;
    }

    public void setParentId(String parentId) {
        this.parentId = parentId;
    }

    public Map<String, String> getProperties() {
        return properties;
    }

    public void setProperties(Map<String, String> properties) {
        this.properties = properties;
    }
}
