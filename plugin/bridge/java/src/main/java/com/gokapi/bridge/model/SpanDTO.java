package com.gokapi.bridge.model;

import com.google.gson.annotations.SerializedName;

/**
 * Wire representation of an inline span (gokapi model.Span).
 */
public class SpanDTO {

    @SerializedName("span_type")
    private int spanType;

    @SerializedName("type")
    private String type;

    @SerializedName("id")
    private String id;

    @SerializedName("data")
    private String data;

    @SerializedName("outer_data")
    private String outerData;

    @SerializedName("deletable")
    private boolean deletable;

    @SerializedName("cloneable")
    private boolean cloneable;

    public int getSpanType() {
        return spanType;
    }

    public void setSpanType(int spanType) {
        this.spanType = spanType;
    }

    public String getType() {
        return type;
    }

    public void setType(String type) {
        this.type = type;
    }

    public String getId() {
        return id;
    }

    public void setId(String id) {
        this.id = id;
    }

    public String getData() {
        return data;
    }

    public void setData(String data) {
        this.data = data;
    }

    public String getOuterData() {
        return outerData;
    }

    public void setOuterData(String outerData) {
        this.outerData = outerData;
    }

    public boolean isDeletable() {
        return deletable;
    }

    public void setDeletable(boolean deletable) {
        this.deletable = deletable;
    }

    public boolean isCloneable() {
        return cloneable;
    }

    public void setCloneable(boolean cloneable) {
        this.cloneable = cloneable;
    }
}
