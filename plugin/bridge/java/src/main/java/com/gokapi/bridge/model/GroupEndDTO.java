package com.gokapi.bridge.model;

import com.google.gson.annotations.SerializedName;

/**
 * Wire representation of a group end event (gokapi model.GroupEnd).
 */
public class GroupEndDTO {

    @SerializedName("id")
    private String id;

    public String getId() {
        return id;
    }

    public void setId(String id) {
        this.id = id;
    }
}
