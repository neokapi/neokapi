package com.gokapi.bridge.model;

import com.google.gson.annotations.SerializedName;

/**
 * Wire representation of a Part (gokapi model.Part / shared.PartDTO).
 * Exactly one of the resource fields is populated based on partType.
 *
 * Part type values:
 * 0 = LayerStart, 1 = LayerEnd, 2 = GroupStart, 3 = GroupEnd,
 * 4 = Block, 5 = Data, 6 = Media
 */
public class PartDTO {

    public static final int TYPE_LAYER_START = 0;
    public static final int TYPE_LAYER_END = 1;
    public static final int TYPE_GROUP_START = 2;
    public static final int TYPE_GROUP_END = 3;
    public static final int TYPE_BLOCK = 4;
    public static final int TYPE_DATA = 5;
    public static final int TYPE_MEDIA = 6;

    @SerializedName("part_type")
    private int partType;

    @SerializedName("block")
    private BlockDTO block;

    @SerializedName("layer")
    private LayerDTO layer;

    @SerializedName("data")
    private DataDTO data;

    @SerializedName("group_start")
    private GroupStartDTO groupStart;

    @SerializedName("group_end")
    private GroupEndDTO groupEnd;

    @SerializedName("media")
    private MediaDTO media;

    public int getPartType() {
        return partType;
    }

    public void setPartType(int partType) {
        this.partType = partType;
    }

    public BlockDTO getBlock() {
        return block;
    }

    public void setBlock(BlockDTO block) {
        this.block = block;
    }

    public LayerDTO getLayer() {
        return layer;
    }

    public void setLayer(LayerDTO layer) {
        this.layer = layer;
    }

    public DataDTO getData() {
        return data;
    }

    public void setData(DataDTO data) {
        this.data = data;
    }

    public GroupStartDTO getGroupStart() {
        return groupStart;
    }

    public void setGroupStart(GroupStartDTO groupStart) {
        this.groupStart = groupStart;
    }

    public GroupEndDTO getGroupEnd() {
        return groupEnd;
    }

    public void setGroupEnd(GroupEndDTO groupEnd) {
        this.groupEnd = groupEnd;
    }

    public MediaDTO getMedia() {
        return media;
    }

    public void setMedia(MediaDTO media) {
        this.media = media;
    }
}
