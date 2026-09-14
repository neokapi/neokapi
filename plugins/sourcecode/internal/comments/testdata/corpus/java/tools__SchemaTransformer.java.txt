package neokapi.bridge.tools;

import com.google.gson.Gson;
import com.google.gson.GsonBuilder;
import com.google.gson.JsonArray;
import com.google.gson.JsonElement;
import com.google.gson.JsonObject;

import java.util.HashSet;
import java.util.Map;
import java.util.Set;

/**
 * Transforms parameter metadata into clean JSON Schema format and merges editor hints.
 * 
 * This class handles:
 * 1. Converting complex Okapi internal formats (like InlineCodeFinder) to clean JSON
 * 2. Merging manually-curated editor hints (groupings, widgets, presets)
 * 3. Adding x-extension properties for UI generation
 */
public class SchemaTransformer {

    private static final Gson GSON = new GsonBuilder().disableHtmlEscaping().create();

    /**
     * Transform a single parameter into JSON Schema property format.
     * 
     * @param paramName The parameter name
     * @param paramInfo The introspected parameter information
     * @return JSON Schema property object
     */
    public JsonObject transformParameter(String paramName, ParameterIntrospector.ParamInfo paramInfo) {
        JsonObject prop = new JsonObject();
        
        // Handle special Okapi formats
        if ("inlineCodeFinder".equals(paramInfo.okapiFormat)) {
            return transformCodeFinderRules();
        }
        
        if ("elementRules".equals(paramInfo.okapiFormat)) {
            return transformElementRules(paramInfo);
        }
        
        if ("attributeRules".equals(paramInfo.okapiFormat)) {
            return transformAttributeRules(paramInfo);
        }
        
        if ("simplifierRules".equals(paramName)) {
            return transformSimplifierRules();
        }
        
        // Skip subfilter parameter - handled via neokapi Layers
        if ("subfilter".equals(paramName)) {
            return null;
        }
        
        // Skip UI-only elements (separators, labels with UUID names)
        if ("separator".equals(paramInfo.widget) || "label".equals(paramInfo.widget)) {
            return null;
        }
        // Also skip parameters with UUID-like names (UI separators)
        if (paramName.matches("[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}")) {
            return null;
        }

        // Basic type mapping
        prop.addProperty("type", paramInfo.type);
        
        // Add default value
        if (paramInfo.defaultValue != null) {
            addDefaultValue(prop, paramInfo.type, paramInfo.defaultValue);
        }
        
        // Add title (displayName) if available
        if (paramInfo.displayName != null && !paramInfo.displayName.isEmpty()) {
            prop.addProperty("title", paramInfo.displayName);
        }
        
        // Add description
        if (paramInfo.description != null && !paramInfo.description.isEmpty()) {
            prop.addProperty("description", paramInfo.description);
        }
        
        // Add enum constraint if applicable
        if (paramInfo.enumValues != null && !paramInfo.enumValues.isEmpty()) {
            JsonArray enumArray = new JsonArray();
            for (String val : paramInfo.enumValues) {
                enumArray.add(val);
            }
            prop.add("enum", enumArray);
            
            // Add enum labels as x-enumLabels if different from values
            if (paramInfo.enumLabels != null && paramInfo.enumLabels.length == paramInfo.enumValues.size()) {
                JsonArray labelsArray = new JsonArray();
                for (String label : paramInfo.enumLabels) {
                    labelsArray.add(label);
                }
                prop.add("x-enumLabels", labelsArray);
            }
        }
        
        // Add numeric constraints for integer types
        if ("integer".equals(paramInfo.type)) {
            if (paramInfo.minimum != null) {
                prop.addProperty("minimum", paramInfo.minimum);
            }
            if (paramInfo.maximum != null) {
                prop.addProperty("maximum", paramInfo.maximum);
            }
        }
        
        // Mark deprecated
        if (paramInfo.deprecated) {
            prop.addProperty("deprecated", true);
        }
        
        // Build structured x-editor object (conforms to x-editor.schema.json).
        // Groups widget-specific properties under their widget type key,
        // mirroring Okapi's EditorDescription / AbstractPart type hierarchy.
        JsonObject editor = buildEditorMetadata(paramInfo);
        if (editor != null) {
            prop.add("x-editor", editor);
        }

        return prop;
    }

    /**
     * Build the structured x-editor metadata object for a parameter.
     * Conforms to x-editor.schema.json. Widget-specific properties are grouped
     * under their widget type key (text, path, folder, checkList), mirroring
     * Okapi EditorDescription AbstractPart subclass hierarchy.
     *
     * Returns null if the parameter has no editor metadata.
     */
    private JsonObject buildEditorMetadata(ParameterIntrospector.ParamInfo paramInfo) {
        if (paramInfo.widget == null && paramInfo.masterParam == null) {
            return null;
        }

        JsonObject editor = new JsonObject();

        // Widget type (discriminator)
        if (paramInfo.widget != null) {
            editor.addProperty("widget", paramInfo.widget);
        }

        // Master/slave dependency (from AbstractPart.getMasterPart())
        if (paramInfo.masterParam != null) {
            JsonObject enabledBy = new JsonObject();
            enabledBy.addProperty("parameter", paramInfo.masterParam);
            enabledBy.addProperty("enabledWhenSelected", paramInfo.enabledOnMasterSelected);
            editor.add("enabledBy", enabledBy);
        }

        // Layout hints from AbstractPart (only emit non-defaults)
        boolean hasLayout = (paramInfo.withLabel != null && !paramInfo.withLabel)
                || (paramInfo.vertical != null && paramInfo.vertical);
        if (hasLayout) {
            JsonObject layout = new JsonObject();
            if (paramInfo.withLabel != null && !paramInfo.withLabel) {
                layout.addProperty("withLabel", false);
            }
            if (paramInfo.vertical != null && paramInfo.vertical) {
                layout.addProperty("vertical", true);
            }
            editor.add("layout", layout);
        }

        // Widget-specific properties grouped under their type key
        // (mirrors Okapi AbstractPart subclass hierarchy)
        if ("text".equals(paramInfo.widget)) {
            JsonObject text = new JsonObject();
            boolean hasProps = false;
            if (paramInfo.password) {
                text.addProperty("password", true);
                hasProps = true;
            }
            if (paramInfo.allowEmpty) {
                text.addProperty("allowEmpty", true);
                hasProps = true;
            }
            if (paramInfo.textHeight != null) {
                text.addProperty("height", paramInfo.textHeight);
                hasProps = true;
            }
            if (hasProps) {
                editor.add("text", text);
            }
        } else if ("path".equals(paramInfo.widget)) {
            JsonObject path = new JsonObject();
            boolean hasProps = false;
            if (paramInfo.browseTitle != null) {
                path.addProperty("browseTitle", paramInfo.browseTitle);
                hasProps = true;
            }
            if (paramInfo.forSaveAs) {
                path.addProperty("forSaveAs", true);
                hasProps = true;
            }
            if (paramInfo.pathAllowEmpty) {
                path.addProperty("allowEmpty", true);
                hasProps = true;
            }
            if (paramInfo.filterNames != null && paramInfo.filterExtensions != null) {
                JsonArray filters = buildFileFilters(paramInfo.filterNames, paramInfo.filterExtensions);
                if (filters.size() > 0) {
                    path.add("filters", filters);
                    hasProps = true;
                }
            }
            if (hasProps) {
                editor.add("path", path);
            }
        } else if ("folder".equals(paramInfo.widget)) {
            if (paramInfo.browseTitle != null) {
                JsonObject folder = new JsonObject();
                folder.addProperty("browseTitle", paramInfo.browseTitle);
                editor.add("folder", folder);
            }
        } else if ("checkList".equals(paramInfo.widget) && paramInfo.checkListEntries != null) {
            JsonObject checkListObj = new JsonObject();
            JsonArray entries = new JsonArray();
            for (ParameterIntrospector.ParamInfo entry : paramInfo.checkListEntries) {
                JsonObject entryObj = new JsonObject();
                entryObj.addProperty("name", entry.name);
                if (entry.displayName != null) {
                    entryObj.addProperty("title", entry.displayName);
                }
                if (entry.description != null) {
                    entryObj.addProperty("description", entry.description);
                }
                entries.add(entryObj);
            }
            checkListObj.add("entries", entries);
            editor.add("checkList", checkListObj);
        }

        return editor;
    }

    /**
     * Parse Okapi tab-separated filter name/extension strings into structured objects.
     * Input: "SRX Documents (*.srx)\tAll Files (*.*)" / "*.srx\t*.*"
     * Output: [{name: "SRX Documents (*.srx)", extensions: "*.srx"}, ...]
     */
    private JsonArray buildFileFilters(String filterNames, String filterExtensions) {
        JsonArray filters = new JsonArray();
        String[] names = filterNames.split("\t");
        String[] exts = filterExtensions.split("\t");
        int count = Math.min(names.length, exts.length);
        for (int i = 0; i < count; i++) {
            if (!names[i].isEmpty() && !exts[i].isEmpty()) {
                JsonObject filter = new JsonObject();
                filter.addProperty("name", names[i]);
                filter.addProperty("extensions", exts[i]);
                filters.add(filter);
            }
        }
        return filters;
    }

    /**
     * Transform InlineCodeFinder to clean object schema.
     *
     * Okapi internal format: "#v1\ncount.i=2\nrule0=<[^>]+>\nrule1=\\{\\d+\\}\nsample=..."
     * Clean neokapi format: { "rules": [{ "pattern": "..." }], "sample": "...", "useAllRulesWhenTesting": true }
     */
    private JsonObject transformCodeFinderRules() {
        JsonObject prop = new JsonObject();
        prop.addProperty("type", "object");
        prop.addProperty("description", "Inline code detection configuration");
        
        JsonObject properties = new JsonObject();
        
        // rules array
        JsonObject rulesArray = new JsonObject();
        rulesArray.addProperty("type", "array");
        rulesArray.addProperty("description", "List of regex patterns to detect inline codes");
        
        JsonObject ruleItem = new JsonObject();
        ruleItem.addProperty("type", "object");
        
        JsonObject ruleProps = new JsonObject();
        JsonObject patternProp = new JsonObject();
        patternProp.addProperty("type", "string");
        patternProp.addProperty("description", "Regex pattern for inline code detection");
        ruleProps.add("pattern", patternProp);
        ruleItem.add("properties", ruleProps);
        
        JsonArray required = new JsonArray();
        required.add("pattern");
        ruleItem.add("required", required);
        
        rulesArray.add("items", ruleItem);
        properties.add("rules", rulesArray);
        
        // sample
        JsonObject sampleProp = new JsonObject();
        sampleProp.addProperty("type", "string");
        sampleProp.addProperty("description", "Sample text to test patterns against");
        properties.add("sample", sampleProp);
        
        // useAllRulesWhenTesting
        JsonObject useAllProp = new JsonObject();
        useAllProp.addProperty("type", "boolean");
        useAllProp.addProperty("default", true);
        useAllProp.addProperty("description", "Test all rules together or individually");
        properties.add("useAllRulesWhenTesting", useAllProp);
        
        prop.add("properties", properties);
        prop.addProperty("x-okapiFormat", "inlineCodeFinder");
        
        return prop;
    }

    /**
     * Transform SimplifierRules to clean schema.
     */
    private JsonObject transformSimplifierRules() {
        JsonObject prop = new JsonObject();
        prop.addProperty("type", "string");
        prop.addProperty("description", "Simplifier rules for code type normalization");
        prop.addProperty("x-widget", "simplifierRulesEditor");
        return prop;
    }

    /**
     * Transform element rules map to a schema with $ref to $defs/elementRule.
     */
    private JsonObject transformElementRules(ParameterIntrospector.ParamInfo paramInfo) {
        JsonObject prop = new JsonObject();
        prop.addProperty("type", "object");
        prop.addProperty("title", "Element Rules");
        prop.addProperty("description", "Element extraction rules — maps element names to their rule configuration");
        prop.addProperty("x-widget", "elementRulesEditor");
        prop.add("additionalProperties", createRef("#/$defs/elementRule"));
        if (paramInfo.defaultValue != null) {
            prop.add("default", GSON.toJsonTree(paramInfo.defaultValue));
        }
        return prop;
    }

    /**
     * Transform attribute rules map to a schema with $ref to $defs/attributeRule.
     */
    private JsonObject transformAttributeRules(ParameterIntrospector.ParamInfo paramInfo) {
        JsonObject prop = new JsonObject();
        prop.addProperty("type", "object");
        prop.addProperty("title", "Attribute Rules");
        prop.addProperty("description", "Global attribute extraction rules — maps attribute names to their rule configuration");
        prop.addProperty("x-widget", "attributeRulesEditor");
        prop.add("additionalProperties", createRef("#/$defs/attributeRule"));
        if (paramInfo.defaultValue != null) {
            prop.add("default", GSON.toJsonTree(paramInfo.defaultValue));
        }
        return prop;
    }

    private JsonObject createRef(String ref) {
        JsonObject obj = new JsonObject();
        obj.addProperty("$ref", ref);
        return obj;
    }

    /**
     * Generate $defs for element and attribute rule sub-schemas.
     * Called by SchemaGenerator for YAML-based filters.
     */
    public JsonObject generateYamlConfigDefs() {
        JsonObject defs = new JsonObject();
        defs.add("elementRule", generateElementRuleDef());
        defs.add("attributeRule", generateAttributeRuleDef());
        defs.add("conditionTuple", generateConditionTupleDef());
        defs.add("conditionalAttributeValue", generateConditionalAttributeValueDef());
        return defs;
    }

    /**
     * Generate a typed condition tuple schema: [attributeName, operator, value(s)].
     */
    private JsonObject generateConditionTupleDef() {
        JsonObject def = new JsonObject();
        def.addProperty("type", "array");
        def.addProperty("description", "Condition: [attributeName, operator, value or values]");
        def.addProperty("minItems", 3);
        def.addProperty("maxItems", 3);

        JsonArray prefixItems = new JsonArray();

        // [0] attribute name
        JsonObject attrName = new JsonObject();
        attrName.addProperty("type", "string");
        attrName.addProperty("description", "Attribute name to test");
        prefixItems.add(attrName);

        // [1] operator
        JsonObject operator = new JsonObject();
        operator.addProperty("type", "string");
        JsonArray opEnum = new JsonArray();
        opEnum.add("EQUALS");
        opEnum.add("NOT_EQUALS");
        opEnum.add("MATCHES");
        operator.add("enum", opEnum);
        JsonObject opDescriptions = new JsonObject();
        opDescriptions.addProperty("EQUALS", "Case-insensitive string equality");
        opDescriptions.addProperty("NOT_EQUALS", "Case-insensitive string inequality");
        opDescriptions.addProperty("MATCHES", "Java regex match (must match entire attribute value)");
        operator.add("x-enumDescriptions", opDescriptions);
        prefixItems.add(operator);

        // [2] value - string or array of strings (OR logic)
        JsonObject value = new JsonObject();
        value.addProperty("description", "Value to compare — a single string or array of strings (OR logic for EQUALS/MATCHES, AND logic for NOT_EQUALS)");
        JsonArray valueOneOf = new JsonArray();
        JsonObject singleStr = new JsonObject();
        singleStr.addProperty("type", "string");
        valueOneOf.add(singleStr);
        JsonObject strArray = new JsonObject();
        strArray.addProperty("type", "array");
        JsonObject strArrItem = new JsonObject();
        strArrItem.addProperty("type", "string");
        strArray.add("items", strArrItem);
        valueOneOf.add(strArray);
        value.add("oneOf", valueOneOf);
        prefixItems.add(value);

        def.add("prefixItems", prefixItems);
        return def;
    }

    private JsonObject generateElementRuleDef() {
        JsonObject def = new JsonObject();
        def.addProperty("type", "object");
        def.addProperty("description", "Extraction rule for an HTML/XML element");

        JsonObject properties = new JsonObject();

        // ruleTypes - array of RULE_TYPE enum values with descriptions
        JsonObject ruleTypes = new JsonObject();
        ruleTypes.addProperty("type", "array");
        ruleTypes.addProperty("title", "Rule Types");
        ruleTypes.addProperty("description", "Extraction rule types for this element");
        JsonObject ruleTypeItems = new JsonObject();
        ruleTypeItems.addProperty("type", "string");
        JsonArray ruleTypeEnum = new JsonArray();
        for (String rt : new String[]{"INLINE", "INLINE_EXCLUDED", "INLINE_INCLUDED",
                "TEXTUNIT", "EXCLUDE", "INCLUDE", "GROUP", "ATTRIBUTES_ONLY",
                "PRESERVE_WHITESPACE", "SCRIPT", "SERVER"}) {
            ruleTypeEnum.add(rt);
        }
        ruleTypeItems.add("enum", ruleTypeEnum);
        JsonObject ruleTypeDescs = new JsonObject();
        ruleTypeDescs.addProperty("INLINE", "Inline element — content flows within surrounding text (e.g. <b>, <span>, <a>)");
        ruleTypeDescs.addProperty("INLINE_EXCLUDED", "Inline element excluded by a conditional rule");
        ruleTypeDescs.addProperty("INLINE_INCLUDED", "Inline element included by a conditional rule (exception to EXCLUDE)");
        ruleTypeDescs.addProperty("TEXTUNIT", "Text unit — extracted as a translatable segment with skeleton before/after");
        ruleTypeDescs.addProperty("EXCLUDE", "Excluded — element and all children are skipped during extraction");
        ruleTypeDescs.addProperty("INCLUDE", "Included — exception to an EXCLUDE rule, re-enables extraction inside excluded block");
        ruleTypeDescs.addProperty("GROUP", "Group element — structural container (e.g. <table>, <ul>, <div>)");
        ruleTypeDescs.addProperty("ATTRIBUTES_ONLY", "Only attributes are translatable/localizable, not the element's text content");
        ruleTypeDescs.addProperty("PRESERVE_WHITESPACE", "Preserve whitespace inside this element (e.g. <pre>, <code>)");
        ruleTypeDescs.addProperty("SCRIPT", "Script element — embedded client-side code (e.g. <script>)");
        ruleTypeDescs.addProperty("SERVER", "Server element — embedded server-side content (e.g. JSP, PHP, Mason tags)");
        ruleTypeItems.add("x-enumDescriptions", ruleTypeDescs);
        ruleTypes.add("items", ruleTypeItems);
        properties.add("ruleTypes", ruleTypes);

        // elementType - optional element type hint
        JsonObject elementType = new JsonObject();
        elementType.addProperty("type", "string");
        elementType.addProperty("title", "Element Type");
        elementType.addProperty("description", "Semantic type hint for UI display (e.g. bold, italic, link, image, paragraph, underlined)");
        properties.add("elementType", elementType);

        // translatableAttributes, writableLocalizableAttributes, readOnlyLocalizableAttributes
        String[][] attrPropDescs = {
            {"translatableAttributes", "Translatable Attributes", "Attributes to extract as translatable content (e.g. alt, title, placeholder)"},
            {"writableLocalizableAttributes", "Writable Localizable Attributes", "Attributes to extract as writable localizable content (e.g. href, src — locale-specific, editable)"},
            {"readOnlyLocalizableAttributes", "Read-Only Localizable Attributes", "Attributes to extract as read-only localizable content (locale-specific but not user-editable)"}
        };
        for (String[] pair : attrPropDescs) {
            JsonObject attr = new JsonObject();
            attr.addProperty("title", pair[1]);
            attr.addProperty("description", pair[2]);
            // Can be a simple string array OR a conditional map
            JsonArray oneOf = new JsonArray();
            // Simple array: ["alt", "title"]
            JsonObject simpleArray = new JsonObject();
            simpleArray.addProperty("type", "array");
            JsonObject strItem = new JsonObject();
            strItem.addProperty("type", "string");
            simpleArray.add("items", strItem);
            oneOf.add(simpleArray);
            // Conditional map: { attrName: [[condAttr, operator, value]] }
            JsonObject condMap = new JsonObject();
            condMap.addProperty("type", "object");
            condMap.add("additionalProperties", createRef("#/$defs/conditionalAttributeValue"));
            oneOf.add(condMap);
            attr.add("oneOf", oneOf);
            properties.add(pair[0], attr);
        }

        // idAttributes - always a simple string array
        JsonObject idAttrs = new JsonObject();
        idAttrs.addProperty("type", "array");
        idAttrs.addProperty("title", "ID Attributes");
        idAttrs.addProperty("description", "Attributes that contain segment IDs (sets the TextUnit name)");
        JsonObject idStrItem = new JsonObject();
        idStrItem.addProperty("type", "string");
        idAttrs.add("items", idStrItem);
        properties.add("idAttributes", idAttrs);

        // conditions - element-level conditions (typed tuple)
        JsonObject elemConds = createRef("#/$defs/conditionTuple");
        elemConds.addProperty("title", "Conditions");
        properties.add("conditions", elemConds);

        def.add("properties", properties);
        // ruleTypes is required
        JsonArray required = new JsonArray();
        required.add("ruleTypes");
        def.add("required", required);
        def.addProperty("additionalProperties", false);
        return def;
    }

    private JsonObject generateAttributeRuleDef() {
        JsonObject def = new JsonObject();
        def.addProperty("type", "object");
        def.addProperty("description", "Global extraction rule for an HTML/XML attribute");

        JsonObject properties = new JsonObject();

        // ruleTypes with descriptions
        JsonObject ruleTypes = new JsonObject();
        ruleTypes.addProperty("type", "array");
        ruleTypes.addProperty("title", "Rule Types");
        ruleTypes.addProperty("description", "Attribute rule types");
        JsonObject ruleTypeItems = new JsonObject();
        ruleTypeItems.addProperty("type", "string");
        JsonArray ruleTypeEnum = new JsonArray();
        for (String rt : new String[]{"ATTRIBUTE_TRANS", "ATTRIBUTE_WRITABLE",
                "ATTRIBUTE_READONLY", "ATTRIBUTE_ID", "ATTRIBUTE_PRESERVE_WHITESPACE"}) {
            ruleTypeEnum.add(rt);
        }
        ruleTypeItems.add("enum", ruleTypeEnum);
        JsonObject attrRuleTypeDescs = new JsonObject();
        attrRuleTypeDescs.addProperty("ATTRIBUTE_TRANS", "Translatable — attribute content is extracted for translation");
        attrRuleTypeDescs.addProperty("ATTRIBUTE_WRITABLE", "Writable localizable — attribute is locale-specific and editable (e.g. URLs, paths)");
        attrRuleTypeDescs.addProperty("ATTRIBUTE_READONLY", "Read-only localizable — attribute is locale-specific but not user-editable");
        attrRuleTypeDescs.addProperty("ATTRIBUTE_ID", "ID — attribute value is used as the segment identifier");
        attrRuleTypeDescs.addProperty("ATTRIBUTE_PRESERVE_WHITESPACE", "Preserve whitespace — attribute controls whitespace preservation state");
        ruleTypeItems.add("x-enumDescriptions", attrRuleTypeDescs);
        ruleTypes.add("items", ruleTypeItems);
        properties.add("ruleTypes", ruleTypes);

        // allElementsExcept
        JsonObject allExcept = new JsonObject();
        allExcept.addProperty("type", "array");
        allExcept.addProperty("title", "All Elements Except");
        allExcept.addProperty("description", "Apply this rule to all elements except the listed ones");
        JsonObject strItem = new JsonObject();
        strItem.addProperty("type", "string");
        allExcept.add("items", strItem);
        properties.add("allElementsExcept", allExcept);

        // onlyTheseElements
        JsonObject onlyThese = new JsonObject();
        onlyThese.addProperty("type", "array");
        onlyThese.addProperty("title", "Only These Elements");
        onlyThese.addProperty("description", "Apply this rule only to the listed elements");
        JsonObject strItem2 = new JsonObject();
        strItem2.addProperty("type", "string");
        onlyThese.add("items", strItem2);
        properties.add("onlyTheseElements", onlyThese);

        // conditions - attribute-level conditions (typed tuple)
        JsonObject attrConds = createRef("#/$defs/conditionTuple");
        attrConds.addProperty("title", "Conditions");
        properties.add("conditions", attrConds);

        // preserve / default - for ATTRIBUTE_PRESERVE_WHITESPACE
        JsonObject preserveCond = createRef("#/$defs/conditionTuple");
        preserveCond.addProperty("title", "Preserve Whitespace Condition");
        preserveCond.addProperty("description", "Condition that activates whitespace preservation (e.g. [xml:space, EQUALS, preserve])");
        properties.add("preserve", preserveCond);

        JsonObject defaultCond = createRef("#/$defs/conditionTuple");
        defaultCond.addProperty("title", "Default Whitespace Condition");
        defaultCond.addProperty("description", "Condition that restores default whitespace handling (e.g. [xml:space, EQUALS, default])");
        properties.add("default", defaultCond);

        def.add("properties", properties);
        JsonArray required = new JsonArray();
        required.add("ruleTypes");
        def.add("required", required);
        def.addProperty("additionalProperties", false);
        return def;
    }

    private JsonObject generateConditionalAttributeValueDef() {
        JsonObject def = new JsonObject();
        def.addProperty("description", "Conditional attribute extraction — null (unconditional) or condition tuple(s)");
        // Can be null (unconditional), a single condition tuple, or array of condition tuples (OR logic)
        JsonArray oneOf = new JsonArray();

        // null — unconditional extraction
        JsonObject nullType = new JsonObject();
        nullType.addProperty("type", "null");
        nullType.addProperty("description", "Extract unconditionally");
        oneOf.add(nullType);

        // Single condition tuple
        oneOf.add(createRef("#/$defs/conditionTuple"));

        // Array of condition tuples (OR logic — extract if any condition matches)
        JsonObject condArrayOfTuples = new JsonObject();
        condArrayOfTuples.addProperty("type", "array");
        condArrayOfTuples.addProperty("description", "Multiple conditions (OR logic — extract if any condition matches)");
        condArrayOfTuples.add("items", createRef("#/$defs/conditionTuple"));
        oneOf.add(condArrayOfTuples);

        def.add("oneOf", oneOf);
        return def;
    }

    /**
     * Add a default value to a property schema.
     */
    private void addDefaultValue(JsonObject prop, String type, Object value) {
        switch (type) {
            case "boolean":
                if (value instanceof Boolean) {
                    prop.addProperty("default", (Boolean) value);
                }
                break;
            case "integer":
                if (value instanceof Number) {
                    prop.addProperty("default", ((Number) value).intValue());
                }
                break;
            case "number":
                if (value instanceof Number) {
                    prop.addProperty("default", ((Number) value).doubleValue());
                }
                break;
            case "string":
                if (value instanceof String) {
                    prop.addProperty("default", (String) value);
                }
                break;
            case "array":
                if (value instanceof JsonArray) {
                    prop.add("default", (JsonArray) value);
                }
                break;
            case "object":
                if (value instanceof JsonObject) {
                    prop.add("default", (JsonObject) value);
                }
                break;
        }
    }

    /**
     * Restructure a flat schema into a hierarchical one using groupings.
     *
     * Groups flat properties into nested objects based on the groupings definition.
     * Common groups (inlineCodes, whitespace) are matched when the filter has at least
     * one of the listed params, emitted as $ref, and their flat params removed.
     * Ungrouped properties remain at root level.
     *
     * @param flatSchema  The flat schema produced by generateBaseSchema
     * @param filterId    Filter ID (e.g. "okf_json")
     * @param groupings   The full groupings.json object
     * @param commonDefs  The common.defs.json object
     * @return The restructured schema (may be the same object if no groupings defined)
     */
    public JsonObject restructureIntoHierarchy(JsonObject flatSchema, String filterId,
                                                JsonObject groupings, JsonObject commonDefs) {
        // No groupings for this filter — return unchanged
        JsonObject filterGrouping = groupings.has(filterId)
                ? groupings.getAsJsonObject(filterId) : null;
        if (filterGrouping == null || filterGrouping.size() == 0) {
            return flatSchema;
        }

        JsonObject flatProperties = flatSchema.has("properties")
                ? flatSchema.getAsJsonObject("properties") : new JsonObject();
        JsonObject newProperties = new JsonObject();
        Set<String> claimedParams = new HashSet<>();

        // Process _common groups first
        JsonObject commonGroups = groupings.has("_common")
                ? groupings.getAsJsonObject("_common") : null;
        JsonObject defs = flatSchema.has("$defs")
                ? flatSchema.getAsJsonObject("$defs").deepCopy() : new JsonObject();
        JsonObject commonDefsSection = commonDefs.has("$defs")
                ? commonDefs.getAsJsonObject("$defs") : new JsonObject();

        if (commonGroups != null) {
            for (Map.Entry<String, JsonElement> entry : commonGroups.entrySet()) {
                String groupName = entry.getKey();
                JsonObject groupDef = entry.getValue().getAsJsonObject();
                JsonArray params = groupDef.getAsJsonArray("params");

                // Check if the filter has at least one of the listed params
                boolean hasAny = false;
                for (JsonElement param : params) {
                    if (flatProperties.has(param.getAsString())) {
                        hasAny = true;
                        break;
                    }
                }
                if (!hasAny) continue;

                // Add $ref at root level
                JsonObject ref = new JsonObject();
                ref.addProperty("$ref", "#/$defs/" + groupName);
                newProperties.add(groupName, ref);

                // Copy the definition from common.defs.json into $defs
                if (commonDefsSection.has(groupName)) {
                    defs.add(groupName, commonDefsSection.get(groupName).deepCopy());
                }
                // Also copy sub-definitions referenced by the common def (e.g. codeFinderRules, simplifierRules)
                if (commonDefsSection.has(groupName) && commonDefsSection.get(groupName).isJsonObject()) {
                    copyReferencedDefs(commonDefsSection.getAsJsonObject(groupName),
                            commonDefsSection, defs);
                }

                // Claim the flat params
                for (JsonElement param : params) {
                    claimedParams.add(param.getAsString());
                }
            }
        }

        // Process filter-specific groups
        for (Map.Entry<String, JsonElement> entry : filterGrouping.entrySet()) {
            String groupName = entry.getKey();
            JsonObject groupDef = entry.getValue().getAsJsonObject();
            String groupTitle = groupDef.has("title")
                    ? groupDef.get("title").getAsString() : null;
            String groupDescription = groupDef.has("description")
                    ? groupDef.get("description").getAsString() : null;
            JsonObject groupProps = groupDef.has("properties")
                    ? groupDef.getAsJsonObject("properties") : new JsonObject();

            JsonObject groupSchema = new JsonObject();
            groupSchema.addProperty("type", "object");
            if (groupTitle != null) {
                groupSchema.addProperty("title", groupTitle);
            }
            if (groupDescription != null) {
                groupSchema.addProperty("description", groupDescription);
            }

            JsonObject groupProperties = new JsonObject();
            for (Map.Entry<String, JsonElement> propEntry : groupProps.entrySet()) {
                String cleanName = propEntry.getKey();
                JsonObject propDef = propEntry.getValue().getAsJsonObject();
                String flattenPath = propDef.has("flattenPath")
                        ? propDef.get("flattenPath").getAsString() : cleanName;

                // Find the original property in the flat schema
                if (flatProperties.has(flattenPath)) {
                    JsonObject originalProp = flatProperties.getAsJsonObject(flattenPath).deepCopy();
                    originalProp.addProperty("x-flattenPath", flattenPath);
                    // Apply title from grouping (short label)
                    if (propDef.has("title") && !originalProp.has("title")) {
                        originalProp.addProperty("title", propDef.get("title").getAsString());
                    }
                    // Apply description from grouping (if provided and not already set)
                    if (propDef.has("description") && !originalProp.has("description")) {
                        originalProp.addProperty("description", propDef.get("description").getAsString());
                    }
                    groupProperties.add(cleanName, originalProp);
                    claimedParams.add(flattenPath);
                }
            }

            if (groupProperties.size() > 0) {
                groupSchema.add("properties", groupProperties);
                newProperties.add(groupName, groupSchema);
            }
        }

        // Add unclaimed properties at root level
        for (Map.Entry<String, JsonElement> entry : flatProperties.entrySet()) {
            String key = entry.getKey();
            if (!claimedParams.contains(key)) {
                // Skip invalid property names (e.g. regex patterns that leaked from default values)
                if (!key.matches("[a-zA-Z_][a-zA-Z0-9_]*")) {
                    System.err.println("[schema-gen] Skipping invalid property name: " + key);
                    continue;
                }
                newProperties.add(key, entry.getValue());
            }
        }

        // Generate x-groups from the restructured properties for UI tab ordering.
        // Each nested object property becomes a group entry. Common groups go first,
        // then filter-specific groups, then any ungrouped properties.
        JsonArray xGroups = new JsonArray();
        for (Map.Entry<String, JsonElement> entry : newProperties.entrySet()) {
            if (entry.getValue().isJsonObject()) {
                JsonObject propObj = entry.getValue().getAsJsonObject();
                if (propObj.has("properties") || propObj.has("$ref")) {
                    JsonObject group = new JsonObject();
                    group.addProperty("id", entry.getKey());
                    // Try title from property, then from $defs if it's a $ref
                    String label = entry.getKey();
                    if (propObj.has("title")) {
                        label = propObj.get("title").getAsString();
                    } else if (propObj.has("$ref")) {
                        String ref = propObj.get("$ref").getAsString();
                        String defName = ref.replace("#/$defs/", "");
                        if (defs.has(defName) && defs.getAsJsonObject(defName).has("title")) {
                            label = defs.getAsJsonObject(defName).get("title").getAsString();
                        }
                    }
                    group.addProperty("label", label);
                    JsonArray fields = new JsonArray();
                    fields.add(entry.getKey());
                    group.add("fields", fields);
                    xGroups.add(group);
                }
            }
        }
        if (xGroups.size() > 0) {
            flatSchema.add("x-groups", xGroups);
        }

        // Update the schema
        flatSchema.add("properties", newProperties);
        if (defs.size() > 0) {
            flatSchema.add("$defs", defs);
        }

        return flatSchema;
    }

    /**
     * Recursively copy $ref-referenced definitions from source defs into target defs.
     */
    private void copyReferencedDefs(JsonObject schema, JsonObject sourceDefs, JsonObject targetDefs) {
        if (schema.has("properties")) {
            JsonObject props = schema.getAsJsonObject("properties");
            for (Map.Entry<String, JsonElement> entry : props.entrySet()) {
                if (!entry.getValue().isJsonObject()) continue;
                JsonObject prop = entry.getValue().getAsJsonObject();
                if (prop.has("$ref")) {
                    String ref = prop.get("$ref").getAsString();
                    // Parse #/$defs/name
                    if (ref.startsWith("#/$defs/")) {
                        String defName = ref.substring("#/$defs/".length());
                        if (sourceDefs.has(defName) && !targetDefs.has(defName)) {
                            targetDefs.add(defName, sourceDefs.get(defName).deepCopy());
                        }
                    }
                }
            }
        }
    }
}
