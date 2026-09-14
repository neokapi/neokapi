package neokapi.bridge.util;

import com.google.gson.JsonArray;
import com.google.gson.JsonElement;
import com.google.gson.JsonObject;
import com.google.gson.JsonPrimitive;
import net.sf.okapi.common.IParameters;
import net.sf.okapi.common.StringParameters;
import net.sf.okapi.common.filters.InlineCodeFinder;

import org.yaml.snakeyaml.DumperOptions;
import org.yaml.snakeyaml.LoaderOptions;
import org.yaml.snakeyaml.Yaml;
import org.yaml.snakeyaml.constructor.Constructor;
import org.yaml.snakeyaml.nodes.NodeId;
import org.yaml.snakeyaml.nodes.Tag;
import org.yaml.snakeyaml.representer.Representer;
import org.yaml.snakeyaml.resolver.Resolver;

import java.lang.reflect.Field;
import java.lang.reflect.Method;
import java.util.*;

/**
 * Applies filter parameters from JSON to Okapi IParameters objects.
 *
 * This class handles:
 * 1. Applying simple parameters via setString/setBoolean/setInteger
 * 2. Applying complex parameters like InlineCodeFinder
 * 3. Using reflection for parameters with dedicated setters
 * 4. YAML merge for markup-based filters (HTML, XML, DITA, etc.)
 */
public class ParameterApplier {

    /**
     * Apply parameters from JSON to an IParameters object.
     *
     * For YAML-based parameters (AbstractMarkupParameters subclasses), complex
     * values like element/attribute rules are applied by merging into the
     * existing YAML configuration. This preserves default rules while allowing
     * selective overrides.
     *
     * @param params The Okapi IParameters object to configure
     * @param filterParams JSON object containing parameter values
     * @return true if all parameters were applied successfully
     */
    public static boolean applyParameters(IParameters params, JsonObject filterParams) {
        if (params == null || filterParams == null) {
            return false;
        }

        // Check if any values are complex (objects/arrays).
        // If so, and params is YAML-based, use YAML merge approach.
        boolean hasComplexValues = false;
        for (Map.Entry<String, JsonElement> entry : filterParams.entrySet()) {
            JsonElement value = entry.getValue();
            if (value != null && !value.isJsonNull() && !value.isJsonPrimitive()) {
                // Skip codeFinderRules — it has its own handler
                if (!"codeFinderRules".equals(entry.getKey())) {
                    hasComplexValues = true;
                    break;
                }
            }
        }

        if (hasComplexValues && isYamlBasedParameters(params)) {
            return applyViaYamlMerge(params, filterParams);
        }

        return applyPerKey(params, filterParams);
    }

    /**
     * Apply parameters one key at a time (original approach).
     */
    private static boolean applyPerKey(IParameters params, JsonObject filterParams) {
        boolean allSuccess = true;

        for (Map.Entry<String, JsonElement> entry : filterParams.entrySet()) {
            String key = entry.getKey();
            JsonElement value = entry.getValue();

            try {
                boolean applied = applyParameter(params, key, value);
                if (!applied) {
                    System.err.println("[bridge] Warning: Could not apply parameter: " + key);
                    allSuccess = false;
                }
            } catch (Exception e) {
                System.err.println("[bridge] Error applying parameter " + key + ": " + e.getMessage());
                allSuccess = false;
            }
        }

        return allSuccess;
    }

    /**
     * Check if the IParameters instance is YAML-based (AbstractMarkupParameters).
     */
    private static boolean isYamlBasedParameters(IParameters params) {
        try {
            Class<?> markupClass = Class.forName(
                    "net.sf.okapi.filters.abstractmarkup.AbstractMarkupParameters");
            return markupClass.isInstance(params);
        } catch (ClassNotFoundException e) {
            return false;
        }
    }

    /**
     * Apply all parameters via YAML merge.
     *
     * This approach:
     * 1. Gets the current YAML config via params.toString()
     * 2. Parses it as a Map
     * 3. Deep-merges the JSON params into the map
     * 4. Serializes back to YAML
     * 5. Applies via params.fromString()
     *
     * Deep merge means: for Map values (like 'elements' and 'attributes'),
     * existing entries are preserved and new/modified entries are overlaid.
     * For scalar values, they are simply overwritten.
     */
    @SuppressWarnings("unchecked")
    private static boolean applyViaYamlMerge(IParameters params, JsonObject filterParams) {
        try {
            // 1. Get current YAML config
            String currentYaml = params.toString();
            if (currentYaml == null || currentYaml.trim().isEmpty()) {
                System.err.println("[bridge] YAML merge: no current config, falling back to per-key");
                return applyPerKey(params, filterParams);
            }

            // 2. Parse as Map using YAML 1.2 boolean rules
            Yaml yaml = createYaml();
            Object parsed = yaml.load(currentYaml);
            Map<String, Object> config;
            if (parsed instanceof Map) {
                config = (Map<String, Object>) parsed;
            } else {
                config = new LinkedHashMap<>();
            }

            // 3. Deep-merge JSON params into the map
            Map<String, Object> overlay = new LinkedHashMap<>();
            for (Map.Entry<String, JsonElement> entry : filterParams.entrySet()) {
                String key = entry.getKey();
                JsonElement value = entry.getValue();

                // Handle codeFinderRules specially — convert to Okapi format first
                if ("codeFinderRules".equals(key) && value.isJsonObject()) {
                    // codeFinderRules uses its own string format, not YAML
                    // Apply it separately after the YAML merge
                    continue;
                }

                overlay.put(key, jsonToJava(value));
            }
            deepMerge(config, overlay);

            // 4. Serialize back to YAML
            Yaml dumper = createYamlDumper();
            String mergedYaml = dumper.dump(config);

            // 5. Apply via fromString()
            params.fromString(mergedYaml);

            System.err.println("[bridge] Applied parameters via YAML merge (" + filterParams.size() + " params)");

            // Handle codeFinderRules separately if present
            if (filterParams.has("codeFinderRules")) {
                applyCodeFinderRules(params, filterParams.get("codeFinderRules"));
            }

            return true;
        } catch (Exception e) {
            System.err.println("[bridge] YAML merge failed: " + e.getMessage());
            e.printStackTrace(System.err);
            // Fall back to per-key approach
            return applyPerKey(params, filterParams);
        }
    }

    /**
     * Deep merge overlay into base map.
     * For nested Maps, entries are merged recursively (preserving existing keys).
     * For all other types, overlay values replace base values.
     */
    @SuppressWarnings("unchecked")
    private static void deepMerge(Map<String, Object> base, Map<String, Object> overlay) {
        for (Map.Entry<String, Object> entry : overlay.entrySet()) {
            String key = entry.getKey();
            Object overlayValue = entry.getValue();
            Object baseValue = base.get(key);

            if (baseValue instanceof Map && overlayValue instanceof Map) {
                deepMerge((Map<String, Object>) baseValue, (Map<String, Object>) overlayValue);
            } else {
                base.put(key, overlayValue);
            }
        }
    }

    /**
     * Convert a JSON element to a Java object suitable for YAML serialization.
     */
    private static Object jsonToJava(JsonElement element) {
        if (element == null || element.isJsonNull()) {
            return null;
        }
        if (element.isJsonPrimitive()) {
            JsonPrimitive p = element.getAsJsonPrimitive();
            if (p.isBoolean()) return p.getAsBoolean();
            if (p.isNumber()) {
                double d = p.getAsDouble();
                if (d == Math.floor(d) && !Double.isInfinite(d)) {
                    long l = p.getAsLong();
                    if (l >= Integer.MIN_VALUE && l <= Integer.MAX_VALUE) {
                        return (int) l;
                    }
                    return l;
                }
                return d;
            }
            return p.getAsString();
        }
        if (element.isJsonObject()) {
            Map<String, Object> map = new LinkedHashMap<>();
            for (Map.Entry<String, JsonElement> e : element.getAsJsonObject().entrySet()) {
                map.put(e.getKey(), jsonToJava(e.getValue()));
            }
            return map;
        }
        if (element.isJsonArray()) {
            List<Object> list = new ArrayList<>();
            for (JsonElement e : element.getAsJsonArray()) {
                list.add(jsonToJava(e));
            }
            return list;
        }
        return null;
    }

    /**
     * Create a SnakeYAML instance with YAML 1.2 boolean rules
     * (only "true"/"false" are booleans, not "yes"/"no"/"on"/"off").
     */
    private static Yaml createYaml() {
        return new Yaml(
                new Constructor(new LoaderOptions()),
                new Representer(new DumperOptions()),
                new DumperOptions(),
                new Yaml12BoolResolver());
    }

    /**
     * Create a SnakeYAML instance configured for dumping with block style.
     */
    private static Yaml createYamlDumper() {
        DumperOptions options = new DumperOptions();
        options.setDefaultFlowStyle(DumperOptions.FlowStyle.BLOCK);
        options.setPrettyFlow(true);
        return new Yaml(
                new Constructor(new LoaderOptions()),
                new Representer(options),
                options,
                new Yaml12BoolResolver());
    }

    /**
     * Custom SnakeYAML Resolver that uses YAML 1.2 boolean rules.
     * In YAML 1.2, only "true" and "false" are booleans.
     */
    private static class Yaml12BoolResolver extends Resolver {
        @Override
        public Tag resolve(NodeId kind, String value, boolean implicit) {
            if (implicit && kind == NodeId.scalar && value != null) {
                String lower = value.toLowerCase();
                if (lower.equals("yes") || lower.equals("no") ||
                    lower.equals("on") || lower.equals("off") ||
                    lower.equals("y") || lower.equals("n")) {
                    return Tag.STR;
                }
            }
            return super.resolve(kind, value, implicit);
        }
    }

    /**
     * Apply a single parameter to an IParameters object.
     */
    private static boolean applyParameter(IParameters params, String key, JsonElement value) {
        if (value == null || value.isJsonNull()) {
            return true; // Null values are OK, just skip
        }

        // Handle codeFinderRules specially
        if ("codeFinderRules".equals(key)) {
            return applyCodeFinderRules(params, value);
        }

        // Handle primitive types via IParameters interface
        if (value.isJsonPrimitive()) {
            if (value.getAsJsonPrimitive().isBoolean()) {
                params.setBoolean(key, value.getAsBoolean());
                return true;
            } else if (value.getAsJsonPrimitive().isNumber()) {
                // Try integer first, then fall back to setting as string
                try {
                    params.setInteger(key, value.getAsInt());
                    return true;
                } catch (Exception e) {
                    // Some parameters might be stored as strings even if they look like numbers
                    params.setString(key, value.getAsString());
                    return true;
                }
            } else {
                params.setString(key, value.getAsString());
                return true;
            }
        }

        // For complex objects, try reflection to find a setter
        return applyViaReflection(params, key, value);
    }

    /**
     * Apply codeFinderRules to the filter parameters.
     */
    private static boolean applyCodeFinderRules(IParameters params, JsonElement value) {
        String okapiFormat;

        if (value.isJsonObject()) {
            // Convert from clean JSON to Okapi format
            okapiFormat = ParameterConverter.convertCodeFinderRules(value.getAsJsonObject());
        } else if (value.isJsonPrimitive()) {
            // Already in Okapi format (or a simple string)
            okapiFormat = value.getAsString();
        } else {
            return false;
        }

        // Try to set via setCodeFinderData method (used by JSON filter)
        try {
            Method setter = params.getClass().getMethod("setCodeFinderData", String.class);
            setter.invoke(params, okapiFormat);
            return true;
        } catch (NoSuchMethodException e) {
            // Try direct field access
        } catch (Exception e) {
            System.err.println("[bridge] Error setting codeFinderData: " + e.getMessage());
        }

        // Try to access the codeFinder field directly
        try {
            Field codeFinderField = findField(params.getClass(), "codeFinder");
            if (codeFinderField != null) {
                codeFinderField.setAccessible(true);
                InlineCodeFinder codeFinder = (InlineCodeFinder) codeFinderField.get(params);
                if (codeFinder != null) {
                    codeFinder.fromString(okapiFormat);
                    return true;
                }
            }
        } catch (Exception e) {
            System.err.println("[bridge] Error accessing codeFinder field: " + e.getMessage());
        }

        // Fall back to setting as a string parameter
        if (params instanceof StringParameters) {
            ((StringParameters) params).setString("codeFinderRules", okapiFormat);
            return true;
        }

        return false;
    }

    /**
     * Try to apply a parameter value using reflection.
     */
    private static boolean applyViaReflection(IParameters params, String key, JsonElement value) {
        // Build setter name
        String setterName = "set" + Character.toUpperCase(key.charAt(0)) + key.substring(1);

        Class<?> paramsClass = params.getClass();

        // Try to find a matching setter
        for (Method method : paramsClass.getMethods()) {
            if (method.getName().equals(setterName) && method.getParameterCount() == 1) {
                Class<?> paramType = method.getParameterTypes()[0];

                try {
                    Object convertedValue = convertToType(value, paramType);
                    if (convertedValue != null) {
                        method.invoke(params, convertedValue);
                        return true;
                    }
                } catch (Exception e) {
                    // Try next method signature
                }
            }
        }

        return false;
    }

    /**
     * Convert a JSON element to the specified Java type.
     */
    private static Object convertToType(JsonElement value, Class<?> targetType) {
        if (value.isJsonPrimitive()) {
            if (targetType == boolean.class || targetType == Boolean.class) {
                return value.getAsBoolean();
            } else if (targetType == int.class || targetType == Integer.class) {
                return value.getAsInt();
            } else if (targetType == long.class || targetType == Long.class) {
                return value.getAsLong();
            } else if (targetType == double.class || targetType == Double.class) {
                return value.getAsDouble();
            } else if (targetType == float.class || targetType == Float.class) {
                return value.getAsFloat();
            } else if (targetType == String.class) {
                return value.getAsString();
            }
        }

        return null;
    }

    /**
     * Find a field in a class or its superclasses.
     */
    private static Field findField(Class<?> clazz, String fieldName) {
        Class<?> current = clazz;
        while (current != null && current != Object.class) {
            try {
                return current.getDeclaredField(fieldName);
            } catch (NoSuchFieldException e) {
                current = current.getSuperclass();
            }
        }
        return null;
    }
}
