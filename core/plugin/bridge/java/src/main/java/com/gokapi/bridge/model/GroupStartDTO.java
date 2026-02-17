package com.gokapi.bridge.model;

import com.google.gson.annotations.SerializedName;

/**
 * Wire representation of a group start event (gokapi model.GroupStart).
 */
public class GroupStartDTO {

    @SerializedName("id")
    private String id;

    @SerializedName("name")
    private String name;

    @SerializedName("type")
    private String type;

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
}
