package com.gokapi.bridge.model;

import com.google.gson.annotations.SerializedName;
import java.util.Map;

/**
 * Wire representation of non-translatable data (gokapi model.Data).
 */
public class DataDTO {

    @SerializedName("id")
    private String id;

    @SerializedName("name")
    private String name;

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

    public Map<String, String> getProperties() {
        return properties;
    }

    public void setProperties(Map<String, String> properties) {
        this.properties = properties;
    }
}
