package neokapi.bridge.util;

import neokapi.bridge.model.FilterConfigurationInfo;
import neokapi.bridge.model.FilterInfo;
import net.sf.okapi.common.IParameters;
import net.sf.okapi.common.filters.FilterConfiguration;
import net.sf.okapi.common.filters.IFilter;
import org.yaml.snakeyaml.Yaml;
import org.yaml.snakeyaml.resolver.Resolver;

import java.io.*;
import java.lang.reflect.Method;
import java.net.URL;
import java.net.URLClassLoader;
import java.nio.charset.StandardCharsets;
import java.util.*;
import java.util.jar.JarEntry;
import java.util.jar.JarFile;

/**
 * Registry of Okapi filters.
 * Dynamically discovers filters by scanning okapi-filter-* JARs on the classpath
 * for classes ending in "Filter" that implement IFilter.

/**
 * Registry of Okapi filters.
 * Dynamically discovers filters by scanning okapi-filter-* JARs on the classpath
 * for classes ending in "Filter" that implement IFilter.
 */
public class FilterRegistry {

    private static final Map<String, FilterInfo> FILTERS = new LinkedHashMap<>();
    private static boolean initialized = false;

    /**
     * Filter IDs that can extract content but cannot faithfully write it back to
     * the original format, so they advertise only the "read" capability. Okapi's
     * PDF filter scrapes text via PDFBox and has no writer that reconstructs a
     * PDF — advertising "write" would let editing tools overwrite the source with
     * plain text.
     */
    private static final Set<String> READ_ONLY_FILTERS = Collections.singleton("okf_pdf");

    /** @return true if {@code filterId} extracts content but cannot write it back. */
    static boolean isReadOnly(String filterId) {
        return READ_ONLY_FILTERS.contains(filterId);
    }

    /**
     * Discover all filters by scanning the classpath for okapi-filter-* JARs.
     * This approach requires no hardcoded filter lists - any filter JAR on the
     * classpath will be automatically discovered.
     */
    private static synchronized void ensureInitialized() {
        if (initialized) {
            return;
        }
        initialized = true;

        Set<String> filterClasses = new TreeSet<>();

        // Scan okapi-filter-* and okapi-lib JARs from the classloader
        // Maven exec:java uses URLClassLoader, not system classpath
        ClassLoader cl = FilterRegistry.class.getClassLoader();
        if (cl instanceof URLClassLoader) {
            for (URL url : ((URLClassLoader) cl).getURLs()) {
                String path = url.getPath();
                if ((path.contains("okapi-filter-") || path.contains("okapi-lib")) && path.endsWith(".jar")) {
                    scanJarForFilters(path, filterClasses);
                }
            }
        }

        // Also check system classpath (for standalone Java runs)
        String classpath = System.getProperty("java.class.path");
        if (classpath != null) {
            for (String path : classpath.split(File.pathSeparator)) {
                if ((path.contains("okapi-filter-") || path.contains("okapi-lib")) && path.endsWith(".jar")) {
                    scanJarForFilters(path, filterClasses);
                }
            }
        }

        // Fallback for shaded/uber JARs: scan the JAR containing this class.
        // In a shaded JAR, all okapi-filter classes are merged into the single
        // uber JAR, so the okapi-filter-* filename scan above finds nothing.
        if (filterClasses.isEmpty()) {
            try {
                java.security.CodeSource cs = FilterRegistry.class.getProtectionDomain().getCodeSource();
                if (cs != null) {
                    File jarFile = new File(cs.getLocation().toURI());
                    if (jarFile.isFile() && jarFile.getName().endsWith(".jar")) {
                        scanJarForFilters(jarFile.getPath(), filterClasses);
                    }
                }
            } catch (Exception e) {
                System.err.println("[bridge] Could not scan uber JAR for filters: " + e.getMessage());
            }
        }

        // Check availability and create FilterInfo for each
        for (String filterClass : filterClasses) {
            FilterInfo info = createFilterInfo(filterClass);
            if (info != null) {
                FILTERS.put(filterClass, info);
            }
        }

        System.err.println("[bridge] Discovered " + FILTERS.size() + " available filters from Okapi");
    }

    /**
     * Scan a JAR file for classes that look like Okapi filters.
     * Filters are classes ending in "Filter" in the net.sf.okapi.filters package.
     */
    private static void scanJarForFilters(String jarPath, Set<String> filterClasses) {
        try (JarFile jar = new JarFile(jarPath)) {
            Enumeration<JarEntry> entries = jar.entries();
            while (entries.hasMoreElements()) {
                JarEntry entry = entries.nextElement();
                String name = entry.getName();
                // Look for Filter classes in net/sf/okapi/filters (not inner classes)
                if (name.startsWith("net/sf/okapi/filters/") 
                        && name.endsWith("Filter.class") 
                        && !name.contains("$")) {
                    String className = name.replace('/', '.').replace(".class", "");
                    filterClasses.add(className);
                }
            }
        } catch (Exception e) {
            System.err.println("[bridge] Could not scan JAR " + jarPath + ": " + e.getMessage());
        }
    }

    /**
     * Create FilterInfo by instantiating the filter and extracting metadata.
     */
    private static FilterInfo createFilterInfo(String filterClass) {
        try {
            Class<?> clazz = Class.forName(filterClass);
            Object instance = clazz.getDeclaredConstructor().newInstance();
            if (!(instance instanceof IFilter)) {
                return null;
            }

            IFilter filter = (IFilter) instance;
            String name = filter.getName();
            String displayName = filter.getDisplayName();

            // Derive format ID from class name (e.g., "HTMLFilter" -> "html")
            String formatId = deriveFormatId(clazz.getSimpleName());

            String filterId = "okf_" + formatId;
            FilterInfo info = new FilterInfo(
                    filterClass,
                    filterId,
                    formatId,
                    displayName != null ? displayName : name,
                    Collections.emptyList(),
                    Collections.emptyList()
            );
            // Most Okapi filters round-trip (read + write); a few are
            // extraction-only and advertise "read" alone (see READ_ONLY_FILTERS).
            info.setCapabilities(isReadOnly(filterId)
                    ? Collections.singletonList("read")
                    : Arrays.asList("read", "write"));

            // Extract filter configurations (presets/variants)
            // For compound filters, track which sibling filter handles each configuration
            Map<String, String> configToFilterClass = new HashMap<>();
            Map<String, String> configToSchemaRef = new HashMap<>();
            
            // Check if filter is a compound filter (may not exist in older Okapi versions)
            if (isCompoundFilter(filter)) {
                populateCompoundFilterConfigMappings(filter, configToFilterClass, configToSchemaRef);
            }
            
            List<FilterConfiguration> configs = filter.getConfigurations();
            if (configs != null && !configs.isEmpty()) {
                boolean firstConfig = true;
                for (FilterConfiguration config : configs) {
                    FilterConfigurationInfo configInfo = new FilterConfigurationInfo(
                            config.configId,
                            config.name,
                            config.description,
                            config.mimeType,
                            config.extensions,
                            config.parametersLocation,
                            firstConfig
                    );
                    
                    // Set the filter class that handles this configuration
                    String handlerClass = configToFilterClass.get(config.configId);
                    if (handlerClass != null) {
                        configInfo.setFilterClass(handlerClass);
                        configInfo.setSchemaRef(configToSchemaRef.get(config.configId));
                    } else {
                        // Regular filter - configuration is handled by the filter itself
                        configInfo.setFilterClass(filterClass);
                    }
                    
                    // Load parameters from file if available
                    if (config.parametersLocation != null && !config.parametersLocation.isEmpty()) {
                        loadParametersForConfig(filter, config.parametersLocation, configInfo);
                    }
                    
                    info.addConfiguration(configInfo);
                    firstConfig = false;
                }
            }

            // Aggregate unique MIME types and extensions from configurations
            Set<String> allMimeTypes = new LinkedHashSet<>();
            Set<String> allExtensions = new LinkedHashSet<>();
            for (FilterConfigurationInfo ci : info.getConfigurations()) {
                if (ci.getMimeType() != null && !ci.getMimeType().isEmpty()) {
                    allMimeTypes.add(ci.getMimeType());
                }
                if (ci.getExtensions() != null && !ci.getExtensions().isEmpty()) {
                    for (String ext : ci.getExtensions().split(";")) {
                        ext = ext.trim();
                        if (!ext.isEmpty()) {
                            allExtensions.add(ext);
                        }
                    }
                }
            }
            info.setMimeTypes(new ArrayList<>(allMimeTypes));
            info.setExtensions(new ArrayList<>(allExtensions));

            return info;
        } catch (Exception e) {
            System.err.println("[bridge] Could not create FilterInfo for " + filterClass + ": " + e.getMessage());
            return null;
        }
    }
    
    /**
     * Check if a filter is a compound filter (AbstractCompoundFilter may not exist in older Okapi versions).
     */
    private static boolean isCompoundFilter(IFilter filter) {
        try {
            Class<?> compoundClass = Class.forName("net.sf.okapi.common.filters.AbstractCompoundFilter");
            return compoundClass.isInstance(filter);
        } catch (ClassNotFoundException e) {
            // AbstractCompoundFilter doesn't exist in this Okapi version
            return false;
        }
    }
    
    /**
     * For compound filters, build a mapping from configId to the sibling filter that handles it.
     * This allows configuration screens to know which schema to use for each configuration.
     */
    private static void populateCompoundFilterConfigMappings(IFilter compoundFilter,
                                                              Map<String, String> configToFilterClass,
                                                              Map<String, String> configToSchemaRef) {
        try {
            Class<?> compoundClass = Class.forName("net.sf.okapi.common.filters.AbstractCompoundFilter");
            
            // Try getting the siblingFilters field directly
            java.lang.reflect.Field siblingField = null;
            try {
                siblingField = compoundClass.getDeclaredField("siblingFilters");
                siblingField.setAccessible(true);
            } catch (NoSuchFieldException e) {
                // Field might have different name
                return;
            }
            
            LinkedList<?> siblings = (LinkedList<?>) siblingField.get(compoundFilter);
            
            if (siblings == null || siblings.isEmpty()) {
                return;
            }
            
            // Each sibling filter contributes its configurations
            for (Object siblingObj : siblings) {
                if (!(siblingObj instanceof IFilter)) continue;
                IFilter sibling = (IFilter) siblingObj;
                
                String siblingClass = sibling.getClass().getName();
                String siblingFormatId = deriveFormatId(sibling.getClass().getSimpleName());
                String schemaRef = "okf_" + siblingFormatId + ".schema.json";
                
                List<FilterConfiguration> siblingConfigs = sibling.getConfigurations();
                if (siblingConfigs != null) {
                    for (FilterConfiguration config : siblingConfigs) {
                        configToFilterClass.put(config.configId, siblingClass);
                        configToSchemaRef.put(config.configId, schemaRef);
                    }
                }
            }
        } catch (Exception e) {
            // Reflection failed - compound filter mappings won't be available
            System.err.println("[bridge] Could not extract sibling filters: " + e.getMessage());
        }
    }

    /**
     * Apply a named built-in Okapi filter configuration to a live filter.
     *
     * <p>Finds the {@link FilterConfiguration} whose {@code configId} equals
     * {@code configId} among {@code filter.getConfigurations()} — for compound
     * filters (e.g. {@code XmlStreamFilter}, {@code XMLFilter}) this includes
     * the built-in presets such as {@code okf_xmlstream-dita},
     * {@code okf_xml-docbook} and {@code okf_xml-resx} — then loads that
     * config's parameters from its {@code parametersLocation} resource (.fprm,
     * .yml, ITS rules, …) and sets them on the filter.
     *
     * <p>The load uses Okapi's own {@code IParameters.load(URL, false)} so the
     * native parameters type parses its native serialization (StringParameters
     * #v1, YAML markup rules, ITS XML, …). The resource is resolved with the
     * same multi-strategy lookup used for schema generation, since a few
     * filters reference parameter files in a parent package.
     *
     * <p>Callers apply this BEFORE any explicit {@code filter_params} so that
     * an explicit override layers on top of the named config's defaults.
     *
     * @param filter   a freshly-created filter instance
     * @param configId the requested Okapi config id (e.g. "okf_xml-resx")
     * @throws IllegalArgumentException if {@code configId} is unknown for the
     *         filter (so a typo surfaces instead of silently using the default)
     * @throws IllegalStateException if the config's parameter resource cannot
     *         be resolved or loaded
     */
    public static void applyNamedConfig(IFilter filter, String configId) {
        if (filter == null) {
            throw new IllegalArgumentException("filter must not be null");
        }
        if (configId == null || configId.isEmpty()) {
            throw new IllegalArgumentException("configId must not be empty");
        }

        List<FilterConfiguration> configs = filter.getConfigurations();
        FilterConfiguration match = null;
        List<String> known = new ArrayList<>();
        if (configs != null) {
            for (FilterConfiguration c : configs) {
                if (c == null) {
                    continue;
                }
                known.add(c.configId);
                if (configId.equals(c.configId)) {
                    match = c;
                    break;
                }
            }
        }

        if (match == null) {
            throw new IllegalArgumentException(
                    "unknown filter configuration '" + configId + "' for "
                            + filter.getClass().getName()
                            + " (available: " + known + ")");
        }

        // The default/base config (e.g. okf_xml, okf_xmlstream) usually carries
        // no parameter resource — the filter's default parameters already match
        // it, so applying it is a no-op rather than an error.
        if (match.parametersLocation == null || match.parametersLocation.isEmpty()) {
            return;
        }

        URL resource = resolveConfigResourceURL(filter, match.parametersLocation);
        if (resource == null) {
            throw new IllegalStateException(
                    "could not resolve parameters resource '" + match.parametersLocation
                            + "' for config '" + configId + "' on "
                            + filter.getClass().getName());
        }

        IParameters params = filter.getParameters();
        if (params == null) {
            throw new IllegalStateException(
                    "filter " + filter.getClass().getName()
                            + " does not support parameters; cannot apply config '" + configId + "'");
        }

        try {
            // Okapi's canonical path: each IParameters implementation parses its
            // own native serialization (StringParameters #v1, AbstractMarkup
            // YAML, ITS rules XML, …). The boolean is ignoreErrors=false.
            params.load(resource, false);
        } catch (Exception e) {
            throw new IllegalStateException(
                    "failed to load parameters for config '" + configId + "' from "
                            + resource + ": " + e.getMessage(), e);
        }
        filter.setParameters(params);
        System.err.println("[bridge] Applied named config '" + configId + "' to "
                + filter.getClass().getName() + " from " + match.parametersLocation);
    }

    /**
     * Resolve a config's {@code parametersLocation} to a {@link URL} using the
     * same multi-strategy lookup as {@link #loadParametersForConfig}. Filters
     * normally reference a resource relative to the filter class, but a few
     * (plaintext, table) reference files in a parent package.
     */
    private static URL resolveConfigResourceURL(IFilter filter, String parametersLocation) {
        // Strategy 1: relative to filter class.
        URL url = filter.getClass().getResource(parametersLocation);

        // Strategy 2: classloader root with absolute path.
        if (url == null) {
            url = filter.getClass().getClassLoader().getResource(parametersLocation);
        }

        // Strategy 3: walk up the filter's package hierarchy.
        if (url == null) {
            String packagePath = filter.getClass().getPackage().getName().replace('.', '/');
            while (url == null && packagePath.contains("/")) {
                packagePath = packagePath.substring(0, packagePath.lastIndexOf('/'));
                url = filter.getClass().getClassLoader().getResource(packagePath + "/" + parametersLocation);
            }
        }

        // Strategy 4: common filter resource location.
        if (url == null) {
            String filterType = extractFilterType(filter.getClass().getName());
            if (filterType != null) {
                url = filter.getClass().getClassLoader().getResource(
                        "net/sf/okapi/filters/" + filterType + "/" + parametersLocation);
            }
        }

        // Strategy 5: plaintext package for okf_plaintext_* configs.
        if (url == null && parametersLocation.startsWith("okf_plaintext")) {
            url = filter.getClass().getClassLoader().getResource(
                    "net/sf/okapi/filters/plaintext/" + parametersLocation);
        }

        // Strategy 6: table package for okf_table_* configs.
        if (url == null && parametersLocation.startsWith("okf_table")) {
            url = filter.getClass().getClassLoader().getResource(
                    "net/sf/okapi/filters/table/" + parametersLocation);
        }

        return url;
    }

    /**
     * Load parameters from a classpath resource file (.yml or .fprm format).
     * Tries multiple resolution strategies since parameter files may be in parent packages.
     */
    private static void loadParametersForConfig(IFilter filter, String parametersLocation,
                                                 FilterConfigurationInfo configInfo) {
        try {
            InputStream is = null;
            
            // Strategy 1: Try relative to filter class (works for most filters)
            is = filter.getClass().getResourceAsStream(parametersLocation);
            
            // Strategy 2: Try from classloader root with absolute path
            if (is == null) {
                is = filter.getClass().getClassLoader().getResourceAsStream(parametersLocation);
            }
            
            // Strategy 3: Try to find in filter's package hierarchy
            // Some filters are in subpackages (e.g., plaintext.base) but reference files
            // in parent package (plaintext)
            if (is == null) {
                String packagePath = filter.getClass().getPackage().getName().replace('.', '/');
                // Try parent packages
                while (is == null && packagePath.contains("/")) {
                    packagePath = packagePath.substring(0, packagePath.lastIndexOf('/'));
                    String fullPath = packagePath + "/" + parametersLocation;
                    is = filter.getClass().getClassLoader().getResourceAsStream(fullPath);
                }
            }
            
            // Strategy 4: Search common filter resource locations
            if (is == null) {
                // Try net/sf/okapi/filters/{filtertype}/ paths
                String filterType = extractFilterType(filter.getClass().getName());
                if (filterType != null) {
                    String fullPath = "net/sf/okapi/filters/" + filterType + "/" + parametersLocation;
                    is = filter.getClass().getClassLoader().getResourceAsStream(fullPath);
                }
            }
            
            // Strategy 5: Try plaintext package for configs that reference okf_plaintext_*
            if (is == null && parametersLocation.startsWith("okf_plaintext")) {
                String fullPath = "net/sf/okapi/filters/plaintext/" + parametersLocation;
                is = filter.getClass().getClassLoader().getResourceAsStream(fullPath);
            }
            
            // Strategy 6: Try table package for configs that reference okf_table_*
            if (is == null && parametersLocation.startsWith("okf_table")) {
                String fullPath = "net/sf/okapi/filters/table/" + parametersLocation;
                is = filter.getClass().getClassLoader().getResourceAsStream(fullPath);
            }
            
            if (is == null) {
                return;
            }
            
            // Read the raw content
            String raw;
            try (BufferedReader reader = new BufferedReader(
                    new InputStreamReader(is, StandardCharsets.UTF_8))) {
                StringBuilder sb = new StringBuilder();
                String line;
                while ((line = reader.readLine()) != null) {
                    sb.append(line).append("\n");
                }
                raw = sb.toString();
            }
            
            configInfo.setParametersRaw(raw);
            
            // Parse based on file extension
            if (parametersLocation.endsWith(".yml") || parametersLocation.endsWith(".yaml")) {
                // Parse YAML with YAML 1.2 boolean rules (yes/no are strings, not booleans)
                Yaml yaml = new Yaml(new org.yaml.snakeyaml.constructor.Constructor(
                        new org.yaml.snakeyaml.LoaderOptions()),
                    new org.yaml.snakeyaml.representer.Representer(
                        new org.yaml.snakeyaml.DumperOptions()),
                    new org.yaml.snakeyaml.DumperOptions(),
                    new Yaml12BoolResolver());
                Object parsed = yaml.load(raw);
                if (parsed instanceof Map) {
                    @SuppressWarnings("unchecked")
                    Map<String, Object> params = (Map<String, Object>) parsed;
                    configInfo.setParameters(params);
                }
            } else if (parametersLocation.endsWith(".fprm")) {
                // Load .fprm via Okapi's own IParameters.fromString(), then extract
                // parameter values. Only extract for filters with known serialization
                // formats (StringParameters or YAML). For opaque configs (XML ITS rules,
                // custom formats), we keep parametersRaw only — the bridge can apply
                // them at runtime via the fprmContent parameter.
                IParameters filterParams = filter.getParameters();
                if (filterParams != null) {
                    // Only extract if the filter uses StringParameters or YAML-based params.
                    // Opaque formats (like XML ITS rules) can't be meaningfully decomposed.
                    boolean isStringParams = filterParams instanceof net.sf.okapi.common.StringParameters;
                    boolean isYamlBased = isAbstractMarkupParameters(filterParams);
                    if (isStringParams || isYamlBased) {
                        com.google.gson.JsonObject json = FprmConverter.toJson(filterParams, raw);
                        if (json != null && json.size() > 0) {
                            Map<String, Object> params = jsonObjectToMap(json);
                            configInfo.setParameters(params);
                        }
                    }
                    // else: opaque config — parametersRaw is already set, no structured parameters
                }
            }
        } catch (Exception e) {
            // Log but don't fail - parameters are optional enhancement
            System.err.println("[bridge] Could not load parameters from " + parametersLocation + ": " + e.getMessage());
        }
    }
    
    /**
     * Check if the IParameters instance uses YAML-based serialization (AbstractMarkupParameters).
     */
    private static boolean isAbstractMarkupParameters(IParameters params) {
        try {
            Class<?> markupClass = Class.forName(
                    "net.sf.okapi.filters.abstractmarkup.AbstractMarkupParameters");
            return markupClass.isInstance(params);
        } catch (ClassNotFoundException e) {
            return false;
        }
    }

    /**
     * Convert a Gson JsonObject to a Map&lt;String, Object&gt; for use as parameters.
     */
    @SuppressWarnings("unchecked")
    private static Map<String, Object> jsonObjectToMap(com.google.gson.JsonObject json) {
        Map<String, Object> map = new LinkedHashMap<>();
        for (Map.Entry<String, com.google.gson.JsonElement> entry : json.entrySet()) {
            map.put(entry.getKey(), jsonElementToObject(entry.getValue()));
        }
        return map;
    }

    /**
     * Convert a Gson JsonElement to a plain Java object.
     */
    @SuppressWarnings("unchecked")
    private static Object jsonElementToObject(com.google.gson.JsonElement element) {
        if (element == null || element.isJsonNull()) {
            return null;
        }
        if (element.isJsonPrimitive()) {
            com.google.gson.JsonPrimitive p = element.getAsJsonPrimitive();
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
            return jsonObjectToMap(element.getAsJsonObject());
        }
        if (element.isJsonArray()) {
            List<Object> list = new ArrayList<>();
            for (com.google.gson.JsonElement e : element.getAsJsonArray()) {
                list.add(jsonElementToObject(e));
            }
            return list;
        }
        return null;
    }
    
    /**
     * Extract filter type from class name (e.g., "plaintext" from 
     * "net.sf.okapi.filters.plaintext.base.BasePlainTextFilter").
     */
    private static String extractFilterType(String className) {
        // Pattern: net.sf.okapi.filters.{filtertype}...
        String prefix = "net.sf.okapi.filters.";
        if (!className.startsWith(prefix)) {
            return null;
        }
        String rest = className.substring(prefix.length());
        int dot = rest.indexOf('.');
        return dot > 0 ? rest.substring(0, dot) : rest;
    }

    /**
     * Derive a format ID from the filter class simple name.
     * E.g., "HTMLFilter" -> "html", "OpenXMLFilter" -> "openxml"
     */
    private static String deriveFormatId(String simpleName) {
        String id = simpleName;
        if (id.endsWith("Filter")) {
            id = id.substring(0, id.length() - 6);
        }
        return id.toLowerCase();
    }

    /**
     * Get metadata for a filter class.
     * Returns cached info if available, otherwise creates it on demand
     * without triggering full filter discovery.
     *
     * @param filterClass fully-qualified Java class name
     * @return FilterInfo or null if not found
     */
    public static FilterInfo getFilterInfo(String filterClass) {
        // Check cache first.
        FilterInfo cached = FILTERS.get(filterClass);
        if (cached != null) {
            return cached;
        }
        // If not in cache and not yet initialized, create info on demand
        // without triggering full discovery.
        if (!initialized && filterClass != null && !filterClass.isEmpty()) {
            FilterInfo info = createFilterInfo(filterClass);
            if (info != null) {
                synchronized (FilterRegistry.class) {
                    FILTERS.put(filterClass, info);
                }
            }
            return info;
        }
        return null;
    }

    /**
     * Create a new instance of the specified filter.
     * This method does NOT trigger full filter discovery — it directly instantiates
     * the class by name, which is much faster than scanning all JARs.
     *
     * @param filterClass fully-qualified Java class name
     * @return new IFilter instance or null
     */
    public static IFilter createFilter(String filterClass) {
        if (filterClass == null || filterClass.isEmpty()) {
            return null;
        }
        // Accept both an Okapi short id ("okf_html") and a fully-qualified
        // Java class name. The short-id path is what neokapi sends — see
        // neokapi#451 for context. Resolution scans the discovered FILTERS
        // (populated by ensureInitialized) for a FilterInfo whose id matches.
        String resolved = resolveFilterClass(filterClass);
        try {
            Class<?> clazz = Class.forName(resolved);
            Object instance = clazz.getDeclaredConstructor().newInstance();
            if (instance instanceof IFilter) {
                return (IFilter) instance;
            }
            return null;
        } catch (Exception e) {
            System.err.println("[bridge] Failed to instantiate filter " + resolved
                    + " (from \"" + filterClass + "\"): " + e.getMessage());
            return null;
        }
    }

    /**
     * Resolve an Okapi short id ("okf_html") or a passthrough FQCN to a
     * Java class name suitable for {@code Class.forName}.
     *
     * Resolution order:
     *   1. If the input contains a dot, treat it as a FQCN unchanged.
     *   2. Otherwise, ensure FILTERS is populated and look up the first
     *      registered FilterInfo whose id equals the input.
     *   3. Fall back to the input unchanged so the caller's error message
     *      surfaces the original string.
     */
    private static String resolveFilterClass(String filterClass) {
        if (filterClass.indexOf('.') >= 0) {
            return filterClass;
        }
        ensureInitialized();
        // Two filters can derive the same short id (e.g. okapi-filter-xliff2's
        // XLIFF2Filter and okapi-filter-rainbowkit's XLIFF2Filter both map to
        // okf_xliff2). The rainbowkit variants are stubs for the Rainbow
        // translation-package workflow — their createFilterWriter() returns
        // null, which breaks the pseudo pipeline. Prefer the non-rainbowkit
        // implementation when there's a tie.
        String fallback = null;
        for (FilterInfo info : FILTERS.values()) {
            if (filterClass.equals(info.getId())) {
                String fqcn = info.getFilterClass();
                if (fqcn != null && fqcn.contains(".rainbowkit.")) {
                    if (fallback == null) {
                        fallback = fqcn;
                    }
                    continue;
                }
                return fqcn;
            }
        }
        return fallback != null ? fallback : filterClass;
    }

    /**
     * List all discovered and available filters.
     */
    public static List<FilterInfo> listFilters() {
        ensureInitialized();
        return new ArrayList<>(FILTERS.values());
    }

    /**
     * Promote a filter id when the input file is a container the requested
     * filter cannot read directly. Used to spare callers from knowing about
     * Okapi's split between content-only inner filters and zip-wrapping
     * outer filters that share the same package.
     *
     * <p>Currently handles {@code okf_odf} (ODFFilter — content.xml / .fodt
     * flat ODF only) being passed a real OpenDocument container
     * (.odt/.ods/.odp/.odg/.ott/.ots/.otp/.otg/.swc/.swx/.sxd/.sxi). In that
     * case, the right filter is {@code okf_openoffice} (OpenOfficeFilter),
     * which unwraps the zip and dispatches to ODFFilter for each inner
     * content.xml/styles.xml/meta.xml. If the input is null, missing, or
     * doesn't match a known mismatch, the input filter id is returned
     * unchanged.
     *
     * @param filterClass requested filter id (Okapi short id or FQCN)
     * @param input       the file the filter will be asked to read
     * @return a possibly-promoted filter id; never null when {@code filterClass} is non-null
     */
    public static String promoteFilterForInput(String filterClass, File input) {
        if (filterClass == null || filterClass.isEmpty() || input == null) {
            return filterClass;
        }
        if ("okf_odf".equals(filterClass) || filterClass.endsWith(".ODFFilter")) {
            if (isOpenOfficeContainer(input)) {
                return "okf_openoffice";
            }
        }
        return filterClass;
    }

    /**
     * Recognise zip-based OpenDocument containers. Matches the extension
     * list OpenOfficeFilter.getConfigurations() advertises, and falls back
     * to a PK\x03\x04 magic-byte sniff so containers with non-standard
     * names still route correctly.
     */
    private static boolean isOpenOfficeContainer(File input) {
        String name = input.getName().toLowerCase(Locale.ROOT);
        if (name.endsWith(".odt") || name.endsWith(".ods") || name.endsWith(".odp")
                || name.endsWith(".odg") || name.endsWith(".ott") || name.endsWith(".ots")
                || name.endsWith(".otp") || name.endsWith(".otg") || name.endsWith(".swc")
                || name.endsWith(".swx") || name.endsWith(".sxd") || name.endsWith(".sxi")) {
            return true;
        }
        if (!input.isFile()) {
            return false;
        }
        try (java.io.InputStream in = new java.io.FileInputStream(input)) {
            byte[] head = new byte[4];
            int n = in.read(head);
            return n == 4 && head[0] == 'P' && head[1] == 'K' && head[2] == 3 && head[3] == 4;
        } catch (Exception ignored) {
            return false;
        }
    }

    /**
     * Get the filter ID (e.g., "okf_html") for a given filter class name.
     * This method does NOT trigger full filter discovery — it derives the ID
     * from the class name if the filter hasn't been discovered yet.
     *
     * @param filterClass fully-qualified Java class name
     * @return filter ID or null if not found
     */
    public static String getFilterId(String filterClass) {
        // Check cache first (populated by ensureInitialized if it ran).
        FilterInfo info = FILTERS.get(filterClass);
        if (info != null && info.getId() != null) {
            return info.getId();
        }
        // Derive from class name without triggering full discovery.
        if (filterClass != null && !filterClass.isEmpty()) {
            String simpleName = filterClass.substring(filterClass.lastIndexOf('.') + 1);
            return "okf_" + deriveFormatId(simpleName);
        }
        return null;
    }

    /**
     * Get all discovered filter class names.
     */
    public static Set<String> getFilterClasses() {
        ensureInitialized();
        return new LinkedHashSet<>(FILTERS.keySet());
    }

    /**
     * Scan for filter class names without instantiating filters or extracting metadata.
     * This is much faster than {@link #getFilterClasses()} because it only reads JAR
     * entries (no class loading, no filter instantiation, no metadata extraction).
     * Use this when you only need class names, e.g., for populating a
     * FilterConfigurationMapper.
     */
    public static Set<String> scanFilterClassNames() {
        Set<String> filterClasses = new TreeSet<>();

        ClassLoader cl = FilterRegistry.class.getClassLoader();
        if (cl instanceof URLClassLoader) {
            for (URL url : ((URLClassLoader) cl).getURLs()) {
                String path = url.getPath();
                if (path.contains("okapi-filter-") && path.endsWith(".jar")) {
                    scanJarForFilters(path, filterClasses);
                }
            }
        }

        String classpath = System.getProperty("java.class.path");
        if (classpath != null) {
            for (String path : classpath.split(File.pathSeparator)) {
                if (path.contains("okapi-filter-") && path.endsWith(".jar")) {
                    scanJarForFilters(path, filterClasses);
                }
            }
        }

        if (filterClasses.isEmpty()) {
            try {
                java.security.CodeSource cs = FilterRegistry.class.getProtectionDomain().getCodeSource();
                if (cs != null) {
                    File jarFile = new File(cs.getLocation().toURI());
                    if (jarFile.isFile() && jarFile.getName().endsWith(".jar")) {
                        scanJarForFilters(jarFile.getPath(), filterClasses);
                    }
                }
            } catch (Exception e) {
                System.err.println("[bridge] Could not scan uber JAR for filter class names: " + e.getMessage());
            }
        }

        return filterClasses;
    }

    /**
     * Resolve a filter class from an envelope Kind string.
     * Format: {@code Okf{Format}FilterConfig} where Format is PascalCase and maps
     * to filter ID {@code okf_{format}}.
     *
     * @param kind e.g. "OkfHtmlFilterConfig"
     * @return the filter class name, or null if not found
     * @throws IllegalArgumentException if the kind format is invalid
     */
    public static String resolveByKind(String kind) {
        ensureInitialized();
        if (kind == null || kind.isEmpty()) {
            return null;
        }

        if (!kind.startsWith("Okf") || !kind.endsWith("FilterConfig")) {
            throw new IllegalArgumentException(
                    "invalid kind format: " + kind + " (expected Okf{Format}FilterConfig)");
        }

        // Extract format: "OkfHtmlFilterConfig" -> "Html" -> "html"
        String format = kind.substring(3, kind.length() - 12); // strip "Okf" and "FilterConfig"
        if (format.isEmpty()) {
            throw new IllegalArgumentException(
                    "invalid kind: " + kind + " (empty format)");
        }
        String formatLower = format.toLowerCase();

        // Map format to filter ID: "html" -> "okf_html"
        String filterId = "okf_" + formatLower;

        // Find the filter class with this filter ID
        for (FilterInfo info : FILTERS.values()) {
            if (filterId.equals(info.getId())) {
                return info.getFilterClass();
            }
        }

        return null;
    }

    /**
     * Compute the Kind string for a filter.
     * Maps filter ID {@code okf_{format}} to {@code Okf{Format}FilterConfig}.
     *
     * @param filterId e.g. "okf_html"
     * @return Kind string e.g. "OkfHtmlFilterConfig"
     */
    public static String toKind(String filterId) {
        if (filterId == null || !filterId.startsWith("okf_")) {
            return null;
        }
        String format = filterId.substring(4); // strip "okf_"
        // PascalCase the format: "html" -> "Html", "openxml" -> "Openxml"
        String pascal = format.substring(0, 1).toUpperCase() + format.substring(1);
        return "Okf" + pascal + "FilterConfig";
    }

    /**
     * Compute the apiVersion string for a filter schema version.
     * The apiVersion is simply {@code vN} where N is the schema version.
     *
     * @param schemaVersion the schema version number (1, 2, etc.)
     * @return apiVersion string e.g. "v1"
     */
    public static String toApiVersion(int schemaVersion) {
        return "v" + schemaVersion;
    }

    /**
     * Custom SnakeYAML Resolver that uses YAML 1.2 boolean rules.
     * In YAML 1.2 only "true" and "false" are booleans, not "yes"/"no"/"on"/"off".
     */
    private static class Yaml12BoolResolver extends Resolver {
        @Override
        public org.yaml.snakeyaml.nodes.Tag resolve(org.yaml.snakeyaml.nodes.NodeId kind,
                String value, boolean implicit) {
            if (implicit && kind == org.yaml.snakeyaml.nodes.NodeId.scalar && value != null) {
                String lower = value.toLowerCase();
                if (lower.equals("yes") || lower.equals("no") ||
                    lower.equals("on") || lower.equals("off") ||
                    lower.equals("y") || lower.equals("n")) {
                    return org.yaml.snakeyaml.nodes.Tag.STR;
                }
            }
            return super.resolve(kind, value, implicit);
        }
    }
}
