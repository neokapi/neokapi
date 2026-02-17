package com.gokapi.bridge.model;

import com.google.gson.annotations.SerializedName;
import java.util.List;

/**
 * Wire representation of target segments for a locale (gokapi shared.TargetDTO).
 */
public class TargetDTO {

    @SerializedName("locale")
    private String locale;

    @SerializedName("segments")
    private List<SegmentDTO> segments;

    public String getLocale() {
        return locale;
    }

    public void setLocale(String locale) {
        this.locale = locale;
    }

    public List<SegmentDTO> getSegments() {
        return segments;
    }

    public void setSegments(List<SegmentDTO> segments) {
        this.segments = segments;
    }
}
