package com.gokapi.bridge.model;

import com.google.gson.annotations.SerializedName;
import java.util.List;
import java.util.Map;

/**
 * Wire representation of a translatable block (gokapi model.Block).
 */
public class BlockDTO {

    @SerializedName("id")
    private String id;

    @SerializedName("name")
    private String name;

    @SerializedName("type")
    private String type;

    @SerializedName("mime_type")
    private String mimeType;

    @SerializedName("translatable")
    private boolean translatable;

    @SerializedName("source")
    private List<SegmentDTO> source;

    @SerializedName("targets")
    private List<TargetDTO> targets;

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

    public String getType() {
        return type;
    }

    public void setType(String type) {
        this.type = type;
    }

    public String getMimeType() {
        return mimeType;
    }

    public void setMimeType(String mimeType) {
        this.mimeType = mimeType;
    }

    public boolean isTranslatable() {
        return translatable;
    }

    public void setTranslatable(boolean translatable) {
        this.translatable = translatable;
    }

    public List<SegmentDTO> getSource() {
        return source;
    }

    public void setSource(List<SegmentDTO> source) {
        this.source = source;
    }

    public List<TargetDTO> getTargets() {
        return targets;
    }

    public void setTargets(List<TargetDTO> targets) {
        this.targets = targets;
    }

    public Map<String, String> getProperties() {
        return properties;
    }

    public void setProperties(Map<String, String> properties) {
        this.properties = properties;
    }
}
