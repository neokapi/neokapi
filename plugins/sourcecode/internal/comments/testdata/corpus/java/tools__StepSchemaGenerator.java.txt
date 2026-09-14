package neokapi.bridge.tools;

import neokapi.bridge.util.StepInfo;

import com.google.gson.Gson;
import com.google.gson.JsonArray;
import com.google.gson.JsonObject;
import com.google.gson.JsonPrimitive;
import net.sf.okapi.common.IParameters;
import net.sf.okapi.common.ParametersDescription;

import java.io.InputStream;
import java.io.InputStreamReader;
import java.lang.annotation.Annotation;
import java.lang.reflect.Method;
import java.nio.charset.StandardCharsets;
import java.util.ArrayList;
import java.util.Collections;
import java.util.List;

/**
 * Generates JSON Schema from Okapi step parameters.
 * Introspects the step's @UsingParameters annotation, instantiates the
 * Parameters class, and parses the serialized ParametersString (#v1 format)
 * to discover parameter names and types.
 *
 * Also extracts step metadata via reflection:
 * - @StepParameterMapping annotations (parameter type mappings)
 * - Event handler methods (overridden handle* methods)
 * - Marker interfaces (e.g., ILoadsResources)
 * - I/O classification based on parameter mappings
 *
 * Build-time only — not part of the runtime bridge plugin.
 */
public class StepSchemaGenerator {

    /** Marker interfaces to check for on step classes. */
    private static final String[] MARKER_INTERFACES = {
            "net.sf.okapi.common.ILoadsResources"
    };

    /** Cached supplementary metadata (loaded lazily from classpath). */
    private static volatile JsonObject helpMetadata;
    private static volatile boolean helpMetadataLoaded;

    /**
     * Generate a JSON Schema object for a step using pure Okapi vocabulary.
     * Produces x-step metadata only — no neokapi-specific extensions.
     *
     * @param info the StepInfo with step metadata
     * @return JSON Schema object, or null if schema cannot be generated
     */
    public static JsonObject generateSchema(StepInfo info) {
        if (info == null) {
            return null;
        }

        String stepId = info.deriveStepId();
        String displayName = info.getName() != null ? info.getName() : info.deriveStepId();

        JsonObject schema = new JsonObject();
        schema.addProperty("$id", stepId);
        schema.addProperty("title", displayName);
        schema.addProperty("type", "object");

        if (info.getDescription() != null && !info.getDescription().isEmpty()) {
            schema.addProperty("description", info.getDescription());
        }

        // x-step metadata (parameter mappings, event handlers, interfaces, I/O classification)
        JsonObject xStep = generateStepMetadata(info);
        if (xStep != null) {
            schema.add("x-step", xStep);
        }

        // Generate properties from the Parameters class.
        JsonObject properties = generateProperties(info);
        if (properties != null && properties.size() > 0) {
            // Enrich from supplementary metadata for properties still missing title/description.
            enrichFromSupplementaryMetadata(properties, stepId);
            schema.add("properties", properties);
        }

        return schema;
    }

    /**
     * Enrich properties with metadata from help-metadata.json (classpath resource).
     * Applies title, description, widget, enables/disables, and enum options.
     * Only fills in values that are missing — never overwrites existing metadata.
     */
    private static void enrichFromSupplementaryMetadata(JsonObject properties, String stepId) {
        JsonObject meta = getHelpMetadata();
        if (meta == null || !meta.has("steps")) return;
        JsonObject steps = meta.getAsJsonObject("steps");
        if (steps == null || !steps.has(stepId)) return;
        JsonObject stepMeta = steps.getAsJsonObject(stepId);
        if (stepMeta == null) return;

        // Check for enriched format with "parameters" key
        if (stepMeta.has("parameters")) {
            stepMeta = stepMeta.getAsJsonObject("parameters");
        }
        if (stepMeta == null) return;

        for (String paramName : properties.keySet()) {
            if (!stepMeta.has(paramName)) continue;
            JsonObject paramMeta = stepMeta.getAsJsonObject(paramName);
            JsonObject prop = properties.getAsJsonObject(paramName);
            if (prop == null || paramMeta == null) continue;

            if (!prop.has("title") && paramMeta.has("title")) {
                prop.addProperty("title", paramMeta.get("title").getAsString());
            }
            if (!prop.has("description") && paramMeta.has("description")) {
                prop.addProperty("description", paramMeta.get("description").getAsString());
            }
            if (!prop.has("x-editor") && paramMeta.has("widget")) {
                JsonObject editor = new JsonObject();
                editor.addProperty("widget", paramMeta.get("widget").getAsString());
                prop.add("x-editor", editor);
            }
            if (paramMeta.has("enables")) {
                prop.add("x-enables", paramMeta.getAsJsonArray("enables"));
            }
        }
    }

    /**
     * Lazily load help-metadata.json from classpath.
     */
    private static JsonObject getHelpMetadata() {
        if (!helpMetadataLoaded) {
            synchronized (StepSchemaGenerator.class) {
                if (!helpMetadataLoaded) {
                    helpMetadata = loadClasspathJson("help-metadata.json");
                    helpMetadataLoaded = true;
                }
            }
        }
        return helpMetadata;
    }

    private static JsonObject loadClasspathJson(String resourceName) {
        InputStream is = StepSchemaGenerator.class.getClassLoader().getResourceAsStream(resourceName);
        if (is == null) {
            return null;
        }
        try (InputStreamReader reader = new InputStreamReader(is, StandardCharsets.UTF_8)) {
            return new Gson().fromJson(reader, JsonObject.class);
        } catch (Exception e) {
            System.err.println("[schema-gen] Failed to load " + resourceName + ": " + e.getMessage());
            return null;
        }
    }

    /**
     * Generate x-step metadata by introspecting the step class via reflection.
     */
    private static JsonObject generateStepMetadata(StepInfo info) {
        JsonObject xStep = new JsonObject();
        xStep.addProperty("class", info.getClassName());

        try {
            Class<?> stepClass = Class.forName(info.getClassName());

            // Extract @StepParameterMapping annotations
            List<String> parameterMappings = extractParameterMappings(stepClass);
            JsonArray mappingsArray = new JsonArray();
            for (String mapping : parameterMappings) {
                mappingsArray.add(mapping);
            }
            xStep.add("parameterMappings", mappingsArray);

            // Extract overridden event handlers
            List<String> eventHandlers = extractEventHandlers(stepClass);
            JsonArray handlersArray = new JsonArray();
            for (String handler : eventHandlers) {
                handlersArray.add(handler);
            }
            xStep.add("eventHandlers", handlersArray);

            // Check marker interfaces
            List<String> interfaces = extractMarkerInterfaces(stepClass);
            JsonArray interfacesArray = new JsonArray();
            for (String iface : interfaces) {
                interfacesArray.add(iface);
            }
            xStep.add("interfaces", interfacesArray);

            // Classify I/O based on parameter mappings and event handlers
            classifyIO(xStep, parameterMappings, eventHandlers);

        } catch (ClassNotFoundException e) {
            System.err.println("[schema-gen] Could not load step class for metadata: " + info.getClassName());
        }

        return xStep;
    }

    /**
     * Extract @StepParameterMapping annotations from the step class and its hierarchy.
     * Uses reflection to avoid compile-time dependency on the annotation class.
     */
    private static List<String> extractParameterMappings(Class<?> stepClass) {
        List<String> mappings = new ArrayList<>();

        try {
            Class<? extends Annotation> spmAnnotation =
                    (Class<? extends Annotation>) Class.forName(
                            "net.sf.okapi.common.pipeline.annotations.StepParameterMapping");
            Method parameterTypeMethod = spmAnnotation.getMethod("parameterType");

            // Check all public methods (includes inherited) for the annotation
            for (Method method : stepClass.getMethods()) {
                Annotation ann = method.getAnnotation(spmAnnotation);
                if (ann != null) {
                    Object paramType = parameterTypeMethod.invoke(ann);
                    String typeName = ((Enum<?>) paramType).name();
                    if (!mappings.contains(typeName)) {
                        mappings.add(typeName);
                    }
                }
            }
        } catch (ClassNotFoundException e) {
            // @StepParameterMapping not available in this Okapi version
        } catch (Exception e) {
            System.err.println("[schema-gen] Error extracting parameter mappings from "
                    + stepClass.getName() + ": " + e.getMessage());
        }

        Collections.sort(mappings);
        return mappings;
    }

    /**
     * Extract overridden event handler methods.
     * Checks step.getClass().getDeclaredMethods() for methods starting with "handle"
     * that override the no-op implementations in BasePipelineStep.
     */
    private static List<String> extractEventHandlers(Class<?> stepClass) {
        List<String> handlers = new ArrayList<>();

        for (Method method : stepClass.getDeclaredMethods()) {
            String name = method.getName();
            if (name.startsWith("handle") && !name.equals("handleEvent")) {
                handlers.add(name);
            }
        }

        Collections.sort(handlers);
        return handlers;
    }

    /**
     * Check if the step implements any notable marker interfaces.
     */
    private static List<String> extractMarkerInterfaces(Class<?> stepClass) {
        List<String> interfaces = new ArrayList<>();

        for (String ifaceName : MARKER_INTERFACES) {
            try {
                Class<?> ifaceClass = Class.forName(ifaceName);
                if (ifaceClass.isAssignableFrom(stepClass)) {
                    // Use simple name for the interface
                    interfaces.add(ifaceClass.getSimpleName());
                }
            } catch (ClassNotFoundException e) {
                // Interface not available in this Okapi version
            }
        }

        return interfaces;
    }

    /**
     * Classify step I/O based on parameter mappings and event handlers.
     */
    private static void classifyIO(JsonObject xStep, List<String> mappings, List<String> handlers) {
        if (mappings.contains("INPUT_RAWDOC")) {
            xStep.addProperty("inputType", "raw-document");
        } else {
            xStep.addProperty("inputType", "filter-events");
        }

        if (mappings.contains("OUTPUT_URI")) {
            xStep.addProperty("outputType", "file");
        } else {
            xStep.addProperty("outputType", "filter-events");
        }
    }

    /**
     * Generate property schemas from the step's Parameters class.
     */
    private static JsonObject generateProperties(StepInfo info) {
        if (info.getParametersClass() == null) {
            return null;
        }

        try {
            Object paramsObj = info.getParametersClass().getDeclaredConstructor().newInstance();
            if (!(paramsObj instanceof IParameters)) {
                return null;
            }
            IParameters params = (IParameters) paramsObj;
            params.reset();

            ParametersDescription paramsDesc = getParametersDescription(info);

            String serialized = params.toString();
            if (serialized == null || serialized.trim().isEmpty()) {
                return null;
            }

            return parseParametersString(serialized, paramsDesc);
        } catch (Exception e) {
            System.err.println("[schema-gen] Could not generate properties for step "
                    + info.getClassName() + ": " + e.getMessage());
            return null;
        }
    }

    private static ParametersDescription getParametersDescription(StepInfo info) {
        if (info.getParametersClass() == null) {
            return null;
        }
        try {
            Object paramsObj = info.getParametersClass().getDeclaredConstructor().newInstance();
            if (paramsObj instanceof IParameters) {
                return ((IParameters) paramsObj).getParametersDescription();
            }
        } catch (Exception e) {
            // Not all parameters provide descriptions
        }
        return null;
    }

    /**
     * Parse a ParametersString (#v1 format) to discover parameter names and types.
     *
     * Detects array-encoded parameters where Okapi serializes a List as indexed
     * flat keys (e.g. usePattern0, usePattern1, ...) with a count key (patternCount).
     * These are collapsed into a single array property with an items schema.
     */
    private static JsonObject parseParametersString(String serialized, ParametersDescription paramsDesc) {
        // Phase 1: Parse all lines into (name, type, defaultValue) tuples
        List<String[]> entries = new ArrayList<>(); // [paramName, type, rawValue]
        for (String line : serialized.split("\n")) {
            line = line.trim();
            if (line.isEmpty() || line.startsWith("#")) continue;
            int eq = line.indexOf('=');
            if (eq < 0) continue;

            String rawKey = line.substring(0, eq).trim();
            String rawValue = line.substring(eq + 1).trim();

            String paramName;
            String paramType;
            if (rawKey.endsWith(".b")) {
                paramName = rawKey.substring(0, rawKey.length() - 2);
                paramType = "boolean";
            } else if (rawKey.endsWith(".i")) {
                paramName = rawKey.substring(0, rawKey.length() - 2);
                paramType = "integer";
            } else {
                paramName = rawKey;
                paramType = "string";
            }
            entries.add(new String[]{paramName, paramType, rawValue});
        }

        // Phase 2: Collapse array-encoded parameters.
        //
        // Okapi's #v1 format encodes List<T> as indexed flat keys:
        //   patternCount.i=8, usePattern0.b=true, sourcePattern0=..., etc.
        //
        // These are known, stable patterns in the Okapi library. We define them
        // explicitly rather than trying to detect them heuristically.
        java.util.Set<String> excludeKeys = new java.util.LinkedHashSet<>();
        java.util.Map<String, JsonObject> arrayProperties = new java.util.LinkedHashMap<>();

        // Known array-encoded parameter groups in Okapi.
        // The collapseCompoundArray method also excludes the bare stems
        // (e.g. "usePattern" without index) and the count key.
        collapseCompoundArray(entries, excludeKeys, arrayProperties,
                "patterns",                                        // output property name
                "patternCount",                                    // count key
                new String[]{"usePattern", "fromSourcePattern", "singlePattern",
                             "severityPattern", "sourcePattern", "targetPattern", "descPattern"},
                new String[]{"boolean",    "boolean",           "boolean",
                             "integer",    "string",            "string",         "string"},
                new String[]{"enabled",    "fromSource",        "single",
                             "severity",   "source",            "target",         "description"});

        collapseSimpleArray(entries, excludeKeys, arrayProperties,
                "extraCodesAllowed", "string");

        collapseSimpleArray(entries, excludeKeys, arrayProperties,
                "missingCodesAllowed", "string");

        // search-and-replace: use0/search0/replace0 with "count" as length
        collapseCompoundArray(entries, excludeKeys, arrayProperties,
                "rules",                                           // output property name
                "count",                                           // count key
                new String[]{"use",     "search",  "replace"},
                new String[]{"string",  "string",  "string"},
                new String[]{"enabled", "search",  "replace"});

        // Phase 3: Emit detected array properties first
        JsonObject properties = new JsonObject();

        // Add detected array properties
        for (java.util.Map.Entry<String, JsonObject> ap : arrayProperties.entrySet()) {
            properties.add(ap.getKey(), ap.getValue());
        }

        // Phase 4: Emit regular (non-indexed) parameters
        for (String[] entry : entries) {
            String paramName = entry[0];
            String paramType = entry[1];
            String rawValue = entry[2];

            // Skip keys consumed by array detection
            if (excludeKeys.contains(paramName)) {
                System.err.println("[schema-gen] Excluded by array detection: " + paramName);
                continue;
            }

            // Skip invalid property names (enum values, file extensions, regex patterns
            // that leak from default values during #v1 serialization)
            if (!paramName.matches("[a-zA-Z][a-zA-Z0-9_]*")) {
                continue;
            }

            // Skip bare stems of collapsed arrays (e.g. "usePattern" without index).
            // These appear as default buffer entries in the #v1 output but are not
            // real parameters — the data lives in the collapsed array property.
            boolean isBareArrayStem = false;
            for (String arrayName : arrayProperties.keySet()) {
                // Array name "patterns" → stems end with "Pattern"
                // Array name "rules" → stems are "use", "search", "replace"
                // Check if this param name matches any stem from a collapsed array
                for (String[] e2 : entries) {
                    if (excludeKeys.contains(e2[0]) && e2[0].startsWith(paramName)
                            && e2[0].length() > paramName.length()
                            && Character.isDigit(e2[0].charAt(paramName.length()))) {
                        isBareArrayStem = true;
                        break;
                    }
                }
                if (isBareArrayStem) break;
            }
            if (isBareArrayStem) continue;

            // Regular parameter
            Object defaultValue;
            if ("boolean".equals(paramType)) {
                defaultValue = Boolean.parseBoolean(rawValue);
            } else if ("integer".equals(paramType)) {
                try { defaultValue = Integer.parseInt(rawValue); }
                catch (NumberFormatException e) { defaultValue = 0; }
            } else {
                defaultValue = rawValue;
            }

            JsonObject prop = new JsonObject();
            prop.addProperty("type", paramType);

            if (defaultValue instanceof Boolean) {
                prop.add("default", new JsonPrimitive((Boolean) defaultValue));
            } else if (defaultValue instanceof Integer) {
                prop.add("default", new JsonPrimitive((Integer) defaultValue));
            } else if (defaultValue instanceof String) {
                prop.add("default", new JsonPrimitive((String) defaultValue));
            }

            if (paramsDesc != null) {
                try {
                    net.sf.okapi.common.ParameterDescriptor pd = paramsDesc.get(paramName);
                    if (pd != null) {
                        String shortDesc = pd.getShortDescription();
                        if (shortDesc != null && !shortDesc.isEmpty()) {
                            prop.addProperty("description", shortDesc);
                        }
                        String displayName = pd.getDisplayName();
                        if (displayName != null && !displayName.isEmpty()) {
                            prop.addProperty("title", displayName);
                        }
                    }
                } catch (Exception e) {
                    // ignore — param may not have a description
                }
            }

            properties.add(paramName, prop);
        }

        return properties;
    }

    /**
     * Collapse a compound array encoded as indexed flat keys.
     * E.g. patternCount=8, usePattern0=..., sourcePattern0=..., usePattern1=..., etc.
     * becomes a single "patterns" array property with object items.
     *
     * Only adds to excludeKeys/arrayProperties if matching keys are actually found.
     */
    private static void collapseCompoundArray(
            List<String[]> entries,
            java.util.Set<String> excludeKeys,
            java.util.Map<String, JsonObject> arrayProperties,
            String arrayName,
            String countKey,
            String[] stems,      // e.g. {"usePattern", "sourcePattern", ...}
            String[] types,      // e.g. {"boolean", "string", ...}
            String[] fieldNames  // e.g. {"enabled", "source", ...}
    ) {
        // Check if any of the indexed keys exist
        boolean found = false;
        for (String[] entry : entries) {
            for (String stem : stems) {
                if (entry[0].startsWith(stem) && entry[0].length() > stem.length()
                        && Character.isDigit(entry[0].charAt(stem.length()))) {
                    found = true;
                    break;
                }
            }
            if (found) break;
        }
        if (!found) return;

        // Exclude count key, all indexed keys, and the bare stems
        excludeKeys.add(countKey);
        for (String stem : stems) {
            excludeKeys.add(stem); // bare stem (e.g. "usePattern" without index)
            System.err.println("[schema-gen] Added stem to exclude: " + stem);
        }
        for (String[] entry : entries) {
            for (String stem : stems) {
                if (entry[0].startsWith(stem) && entry[0].length() > stem.length()
                        && Character.isDigit(entry[0].charAt(stem.length()))) {
                    excludeKeys.add(entry[0]);
                }
            }
        }

        // Build items schema
        JsonObject itemsProps = new JsonObject();
        for (int i = 0; i < stems.length; i++) {
            JsonObject fieldSchema = new JsonObject();
            fieldSchema.addProperty("type", types[i]);
            itemsProps.add(fieldNames[i], fieldSchema);
        }

        JsonObject items = new JsonObject();
        items.addProperty("type", "object");
        items.add("properties", itemsProps);

        JsonObject arrayProp = new JsonObject();
        arrayProp.addProperty("type", "array");
        arrayProp.add("items", items);
        arrayProp.addProperty("description", "Array of " + arrayName + " encoded from indexed flat parameters.");

        arrayProperties.put(arrayName, arrayProp);
    }

    /**
     * Collapse a simple array encoded as indexed flat keys.
     * E.g. extraCodesAllowed=0 (count), extraCodesAllowed0=..., extraCodesAllowed1=...
     * becomes a single "extraCodesAllowed" array property with string items.
     */
    private static void collapseSimpleArray(
            List<String[]> entries,
            java.util.Set<String> excludeKeys,
            java.util.Map<String, JsonObject> arrayProperties,
            String stem,
            String itemType
    ) {
        boolean found = false;
        for (String[] entry : entries) {
            if (entry[0].startsWith(stem) && entry[0].length() > stem.length()
                    && Character.isDigit(entry[0].charAt(stem.length()))) {
                found = true;
                excludeKeys.add(entry[0]);
            }
        }
        if (!found) return;

        // The stem itself is the count key (integer)
        excludeKeys.add(stem);

        JsonObject items = new JsonObject();
        items.addProperty("type", itemType);

        JsonObject arrayProp = new JsonObject();
        arrayProp.addProperty("type", "array");
        arrayProp.add("items", items);

        arrayProperties.put(stem, arrayProp);
    }
}
