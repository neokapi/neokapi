package com.gokapi.bridge.model;

import com.google.gson.annotations.SerializedName;

/**
 * Wire representation of a segment (gokapi model.Segment).
 */
public class SegmentDTO {

    @SerializedName("id")
    private String id;

    @SerializedName("content")
    private FragmentDTO content;

    public String getId() {
        return id;
    }

    public void setId(String id) {
        this.id = id;
    }

    public FragmentDTO getContent() {
        return content;
    }

    public void setContent(FragmentDTO content) {
        this.content = content;
    }
}
