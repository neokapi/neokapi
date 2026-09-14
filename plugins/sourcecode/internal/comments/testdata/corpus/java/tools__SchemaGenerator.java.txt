package neokapi.bridge.tools;

import neokapi.bridge.model.FilterInfo;
import neokapi.bridge.util.FilterRegistry;
import neokapi.bridge.util.StepInfo;
import neokapi.bridge.util.StepRegistry;
import com.google.gson.Gson;
import com.google.gson.GsonBuilder;
import com.google.gson.JsonObject;

import java.io.File;
import java.io.FileWriter;
import java.io.IOException;
import java.io.InputStream;
import java.io.InputStreamReader;
import java.nio.charset.StandardCharsets;
import java.util.List;
import java.util.Map;

/**
 * Generates base JSON Schema files for Okapi filters.
 * 
 * This tool introspects filter Parameters classes at build time using reflection,
 * then transforms the extracted parameter metadata into clean JSON Schema (draft-07).
 * 
 * Output is base schemas only - merging with overrides is done by bash/jq scripts.
 * 
 * Run via: mvn exec:java -Dexec.mainClass=neokapi.bridge.schema.SchemaGenerator
 */
public class SchemaGenerator {

    private static final Gson GSON = new GsonBuilder()
            .setPrettyPrinting()
            .disableHtmlEscaping()
            .create();

    private static final String OUTPUT_DIR = "schemas";

    private final ParameterIntrospector introspector;
    private final SchemaTransformer transformer;
    private final JsonObject groupings;
    private final JsonObject commonDefs;
    private final JsonObject resMetadata;
    private final JsonObject helpMetadata;

    // Internal implementation fields that leak from introspection but are not
    // user-facing configuration parameters. Excluded from both filter and step schemas.
    private static final java.util.Set<String> EXCLUDED_PROPERTIES = java.util.Set.of(
            // Array stems (collapsed into "patterns" array)
            "usePattern", "fromSourcePattern", "singlePattern", "severityPattern",
            "sourcePattern", "targetPattern", "descPattern", "patternCount",
            // Leaked enum values
            "None", "Transitional", "Strict",                    // xliffSchemaType values
            "tmx", "po", "table", "pensieve", "corpus",         // format-conversion format constants
            "wordtable", "xliff",                                // format-conversion format constants
            "original", "generic", "plain",                      // inconsistency-check display options
            "translation_type", "translation_status",            // xliff-splitter IWS attribute names
            ".qccfg",                                            // quality-check file extension constant
            // AbstractMarkupParameters internal fields (not user-facing config)
            "editorTitle", "taggedConfig");

    public SchemaGenerator() {
        this.introspector = new ParameterIntrospector();
        this.transformer = new SchemaTransformer();
        this.groupings = loadClasspathJson("groupings.json");
        this.commonDefs = loadClasspathJson("common.defs.json");
        this.resMetadata = loadClasspathJson("res-metadata.json");
        this.helpMetadata = loadClasspathJson("help-metadata.json");
    }

    private JsonObject loadClasspathJson(String resourceName) {
        InputStream is = getClass().getClassLoader().getResourceAsStream(resourceName);
        if (is == null) {
            System.err.println("Warning: " + resourceName + " not found on classpath, hierarchy disabled");
            return new JsonObject();
        }
        try (InputStreamReader reader = new InputStreamReader(is, StandardCharsets.UTF_8)) {
            return GSON.fromJson(reader, JsonObject.class);
        } catch (IOException e) {
            System.err.println("Warning: Failed to load " + resourceName + ": " + e.getMessage());
            return new JsonObject();
        }
    }

    public static void main(String[] args) {
        SchemaGenerator generator = new SchemaGenerator();
        
        String outputDir = OUTPUT_DIR;
        if (args.length > 0) {
            outputDir = args[0];
        }

        try {
            generator.generateAll(outputDir);
        } catch (Exception e) {
            System.err.println("Schema generation failed: " + e.getMessage());
            e.printStackTrace();
            System.exit(1);
        }
    }

    /**
     * Generate base schemas for all discovered filters.
     */
    public void generateAll(String outputDir) throws IOException {
        File dir = new File(outputDir);
        if (!dir.exists() && !dir.mkdirs()) {
            throw new IOException("Failed to create output directory: " + outputDir);
        }

        List<FilterInfo> filters = FilterRegistry.listFilters();
        int successCount = 0;
        int failCount = 0;

        System.out.println("Generating base schemas for " + filters.size() + " filters...\n");

        for (FilterInfo info : filters) {
            try {
                JsonObject schema = generateBaseSchema(info);
                String filterId = "okf_" + info.getName();
                String filename = filterId + ".schema.json";
                File outputFile = new File(dir, filename);
                
                try (FileWriter writer = new FileWriter(outputFile, StandardCharsets.UTF_8)) {
                    GSON.toJson(schema, writer);
                }
                
                int paramCount = schema.getAsJsonObject("properties").size();
                System.out.println("✓ " + filterId + " (" + paramCount + " params)");
                successCount++;
            } catch (Exception e) {
                System.err.println("✗ " + info.getName() + " → " + e.getMessage());
                failCount++;
            }
        }

        System.out.println("\nGeneration complete: " + successCount + " schemas, " + failCount + " failures");
        generateMetaFile(dir, filters);

        // Generate step schemas
        generateStepSchemas(outputDir);
    }

    /**
     * Generate JSON Schema files for all discovered Okapi pipeline steps.
     *
     * Uses StepSchemaGenerator for step metadata (x-step, x-component, I/O
     * classification), then enriches the properties with ParameterIntrospector
     * which extracts richer metadata from ParametersDescription and
     * IEditorDescriptionProvider (widgets, enum dropdowns, min/max constraints,
     * master/slave relationships).
     */
    private void generateStepSchemas(String outputDir) throws IOException {
        File stepsDir = new File(outputDir, "steps");
        if (!stepsDir.exists() && !stepsDir.mkdirs()) {
            throw new IOException("Failed to create step schemas directory: " + stepsDir);
        }

        List<StepInfo> steps = StepRegistry.listSteps();
        int successCount = 0;
        int failCount = 0;

        System.out.println("\nGenerating step schemas for " + steps.size() + " steps...\n");

        for (StepInfo step : steps) {
            try {
                JsonObject schema = StepSchemaGenerator.generateSchema(step);
                if (schema == null) {
                    System.err.println("\u2717 " + step.getName() + " \u2192 null schema");
                    failCount++;
                    continue;
                }

                // Enrich properties via ParameterIntrospector if the step has a
                // Parameters class. This extracts descriptions, widgets, enums,
                // min/max, and master/slave relationships that the basic #v1 parser
                // in StepSchemaGenerator does not capture.
                enrichStepProperties(schema, step);

                String filename = step.getStepId() + ".schema.json";
                File outputFile = new File(stepsDir, filename);

                try (FileWriter writer = new FileWriter(outputFile, StandardCharsets.UTF_8)) {
                    GSON.toJson(schema, writer);
                }

                int paramCount = schema.has("properties") ? schema.getAsJsonObject("properties").size() : 0;
                System.out.println("\u2713 " + step.getStepId() + " (" + paramCount + " params)");
                successCount++;
            } catch (Exception e) {
                System.err.println("\u2717 " + step.getName() + " \u2192 " + e.getMessage());
                failCount++;
            }
        }

        System.out.println("\nStep generation complete: " + successCount + " schemas, " + failCount + " failures");
    }

    /**
     * Enrich a step schema's properties using ParameterIntrospector.
     *
     * The introspector extracts richer metadata than the #v1 string parser:
     * - Descriptions and display names from ParametersDescription
     * - Widget types from IEditorDescriptionProvider (checkbox, spin, dropdown, path, folder, codeFinder)
     * - Enum values and labels from ListSelectionPart
     * - Min/max constraints from SpinInputPart
     * - Master/slave (x-enabledBy) relationships from AbstractPart.getMasterPart()
     */
    private void enrichStepProperties(JsonObject schema, StepInfo step) {
        if (step.getParametersClass() == null) {
            return;
        }

        Map<String, ParameterIntrospector.ParamInfo> params = introspector.introspectParamsClass(step.getParametersClass());
        if (params == null || params.isEmpty()) {
            return;
        }

        // Get existing properties from the basic #v1 parse (has defaults and basic types).
        JsonObject existingProps = schema.has("properties")
                ? schema.getAsJsonObject("properties") : new JsonObject();

        // Known bare stems from array-encoded parameters (collapsed by StepSchemaGenerator).
        // These appear in introspection but are serialization artifacts, not real parameters.
        // Use the class-level exclude set for internal implementation fields

        // Build enriched properties using the transformer.
        JsonObject enrichedProps = new JsonObject();
        for (Map.Entry<String, ParameterIntrospector.ParamInfo> entry : params.entrySet()) {
            String paramName = entry.getKey();
            if (EXCLUDED_PROPERTIES.contains(paramName)) continue;
            ParameterIntrospector.ParamInfo paramInfo = entry.getValue();

            JsonObject propSchema = transformer.transformParameter(paramName, paramInfo);
            if (propSchema == null) {
                continue;
            }

            // Preserve the default value from the existing #v1 parse if the
            // introspector didn't find one (the #v1 defaults are authoritative
            // since they come from params.reset() + toString()).
            if (!propSchema.has("default") && existingProps.has(paramName)) {
                JsonObject existing = existingProps.getAsJsonObject(paramName);
                if (existing.has("default")) {
                    propSchema.add("default", existing.get("default"));
                }
            }

            enrichedProps.add(paramName, propSchema);
        }

        // Include any params from #v1 that introspection missed (e.g. nested
        // keys like "codeFinderRules.rule0" that the introspector doesn't see).
        // Skip array properties and invalid property names.
        for (Map.Entry<String, com.google.gson.JsonElement> entry : existingProps.entrySet()) {
            String key = entry.getKey();
            if (!enrichedProps.has(key)) {
                // Skip invalid names and array-collapsed properties (patterns, rules arrays)
                if (!key.matches("[a-zA-Z_][a-zA-Z0-9_]*")) continue;
                enrichedProps.add(key, entry.getValue());
            }
        }

        if (enrichedProps.size() > 0) {
            schema.add("properties", enrichedProps);
        }
    }

    /**
     * Generate a base schema for a single filter (no override merging).
     */
    public JsonObject generateBaseSchema(FilterInfo info) {
        String filterClass = info.getFilterClass();
        String filterId = "okf_" + info.getName();

        JsonObject schema = new JsonObject();
        schema.addProperty("$schema", "http://json-schema.org/draft-07/schema#");
        schema.addProperty("title", info.getDisplayName());
        schema.addProperty("type", "object");

        JsonObject filterMeta = new JsonObject();
        filterMeta.addProperty("id", filterId);
        filterMeta.addProperty("class", filterClass);
        filterMeta.add("extensions", GSON.toJsonTree(info.getExtensions()));
        filterMeta.add("mimeTypes", GSON.toJsonTree(info.getMimeTypes()));
        
        // Include filter configurations (presets/variants)
        if (info.getConfigurations() != null && !info.getConfigurations().isEmpty()) {
            filterMeta.add("configurations", GSON.toJsonTree(info.getConfigurations()));
        }
        
        // Include serialization format
        filterMeta.addProperty("serializationFormat", introspector.getSerializationFormat(filterClass));
        
        schema.add("x-filter", filterMeta);

        Map<String, ParameterIntrospector.ParamInfo> params = introspector.introspect(filterClass);

        JsonObject properties = new JsonObject();
        boolean needsYamlDefs = false;
        if (params != null) {
            for (Map.Entry<String, ParameterIntrospector.ParamInfo> entry : params.entrySet()) {
                String paramName = entry.getKey();
                ParameterIntrospector.ParamInfo paramInfo = entry.getValue();

                // Skip internal implementation fields (not user-facing config)
                if (EXCLUDED_PROPERTIES.contains(paramName)) continue;
                // Skip invalid property names
                if (!paramName.matches("[a-zA-Z][a-zA-Z0-9_]*")) continue;

                if ("elementRules".equals(paramInfo.okapiFormat) || "attributeRules".equals(paramInfo.okapiFormat)) {
                    needsYamlDefs = true;
                }

                JsonObject propSchema = transformer.transformParameter(paramName, paramInfo);
                if (propSchema != null) {
                    properties.add(paramName, propSchema);
                }
            }
        }
        schema.add("properties", properties);
        schema.addProperty("additionalProperties", false);

        // Enrich properties missing title/description from supplementary metadata
        enrichFromSupplementaryMetadata(properties, filterId, "filters");

        // Add $defs for YAML-config filters that use element/attribute rules
        if (needsYamlDefs) {
            schema.add("$defs", transformer.generateYamlConfigDefs());
        }

        // Restructure flat properties into hierarchy using groupings
        schema = transformer.restructureIntoHierarchy(schema, filterId, groupings, commonDefs);

        return schema;
    }

    /**
     * Enrich schema properties with metadata from supplementary files.
     * Priority: res-metadata.json first, then help-metadata.json.
     * Only fills in values that are missing — never overwrites existing metadata.
     *
     * Applies: title, description, x-editor (widget), x-showIf (enables/disables),
     * and enum options from the enriched res-metadata format.
     */
    private void enrichFromSupplementaryMetadata(JsonObject properties, String componentId, String category) {
        if (properties == null || properties.size() == 0) return;

        // Try res-metadata first (has widget/group/enables), then help-metadata (title/desc only)
        JsonObject[] sources = { resMetadata, helpMetadata };
        for (JsonObject source : sources) {
            if (source == null || !source.has(category)) continue;
            JsonObject categoryObj = source.getAsJsonObject(category);
            if (categoryObj == null || !categoryObj.has(componentId)) continue;

            // res-metadata uses { groups: [...], parameters: {...} } format
            // help-metadata uses flat { paramName: {...} } format
            JsonObject paramsMeta = categoryObj.getAsJsonObject(componentId);
            if (paramsMeta == null) continue;

            // Check if this is the enriched format with "parameters" key
            if (paramsMeta.has("parameters")) {
                paramsMeta = paramsMeta.getAsJsonObject("parameters");
            }
            if (paramsMeta == null) continue;

            for (String paramName : properties.keySet()) {
                if (!paramsMeta.has(paramName)) continue;
                JsonObject meta = paramsMeta.getAsJsonObject(paramName);
                JsonObject prop = properties.getAsJsonObject(paramName);
                if (prop == null || meta == null) continue;

                // Title and description (never overwrite)
                if (!prop.has("title") && meta.has("title")) {
                    prop.addProperty("title", meta.get("title").getAsString());
                }
                if (!prop.has("description") && meta.has("description")) {
                    prop.addProperty("description", meta.get("description").getAsString());
                }

                // Widget type → x-editor
                if (!prop.has("x-editor") && meta.has("widget")) {
                    JsonObject editor = new JsonObject();
                    editor.addProperty("widget", meta.get("widget").getAsString());
                    prop.add("x-editor", editor);
                }

                // Conditional visibility (enables/disables)
                if (meta.has("enables") && !prop.has("x-showIf")) {
                    prop.add("x-enables", meta.getAsJsonArray("enables"));
                }
                if (meta.has("disables")) {
                    prop.add("x-disables", meta.getAsJsonArray("disables"));
                }

                // Enum options from radio/select widgets
                if (meta.has("options") && !prop.has("x-presets")) {
                    prop.add("x-options", meta.getAsJsonArray("options"));
                }
            }
        }
    }

    /**
     * Generate meta.json with filter metadata.
     */
    private void generateMetaFile(File dir, List<FilterInfo> filters) throws IOException {
        File metaFile = new File(dir, "meta.json");
        
        JsonObject meta = new JsonObject();
        meta.addProperty("generatedAt", java.time.Instant.now().toString());
        meta.addProperty("filterCount", filters.size());

        JsonObject filtersObj = new JsonObject();
        for (FilterInfo info : filters) {
            String filterId = "okf_" + info.getName();
            JsonObject filterMeta = new JsonObject();
            filterMeta.addProperty("class", info.getFilterClass());
            filterMeta.addProperty("displayName", info.getDisplayName());
            filterMeta.add("extensions", GSON.toJsonTree(info.getExtensions()));
            filterMeta.add("mimeTypes", GSON.toJsonTree(info.getMimeTypes()));
            
            // Include filter configurations
            if (info.getConfigurations() != null && !info.getConfigurations().isEmpty()) {
                filterMeta.add("configurations", GSON.toJsonTree(info.getConfigurations()));
            }
            filtersObj.add(filterId, filterMeta);
        }
        meta.add("filters", filtersObj);

        try (FileWriter writer = new FileWriter(metaFile, StandardCharsets.UTF_8)) {
            GSON.toJson(meta, writer);
        }
        
        System.out.println("Generated meta.json");
    }
}
