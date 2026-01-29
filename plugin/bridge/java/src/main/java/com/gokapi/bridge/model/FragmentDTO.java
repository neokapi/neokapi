package com.gokapi.bridge.model;

import com.google.gson.annotations.SerializedName;
import java.util.List;

/**
 * Wire representation of a text fragment with inline spans (gokapi model.Fragment).
 */
public class FragmentDTO {

    @SerializedName("coded_text")
    private String codedText;

    @SerializedName("spans")
    private List<SpanDTO> spans;

    public String getCodedText() {
        return codedText;
    }

    public void setCodedText(String codedText) {
        this.codedText = codedText;
    }

    public List<SpanDTO> getSpans() {
        return spans;
    }

    public void setSpans(List<SpanDTO> spans) {
        this.spans = spans;
    }
}
