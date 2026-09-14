package neokapi.bridge.tools;

import net.sf.okapi.common.IParameters;
import net.sf.okapi.common.ISimplifierRulesParameters;
import net.sf.okapi.common.ParameterDescriptor;
import net.sf.okapi.common.ParametersDescription;
import net.sf.okapi.common.StringParameters;
import net.sf.okapi.common.filters.IFilter;
import net.sf.okapi.common.filters.InlineCodeFinder;
import net.sf.okapi.common.uidescription.*;

import org.yaml.snakeyaml.Yaml;
import org.yaml.snakeyaml.resolver.Resolver;

import java.lang.reflect.Field;
import java.lang.reflect.Method;
import java.lang.reflect.Modifier;
import java.util.*;

/**
 * Introspects Okapi filter Parameters classes to extract parameter metadata.
 * 
 * This handles three types of parameter patterns used in Okapi:
 * 1. StringParameters subclasses - key-value based with typed accessors
 * 2. AbstractMarkupParameters - YAML-based TaggedFilterConfiguration
 * 3. Direct field access - public fields or getter/setter pairs
 * 
 * Also extracts descriptions from:
 * - getParametersDescription() method
 * - IEditorDescriptionProvider.createEditorDescription() for UI metadata
 * - ISimplifierRulesParameters interface constants
 */
public class ParameterIntrospector {

    /**
     * Information about a single parameter.
     */
    public static class ParamInfo {
        public String name;
        public String type;          // "boolean", "string", "integer", "object", "array"
        public Object defaultValue;
        public String description;
        public String displayName;
        public boolean deprecated;
        public String okapiFormat;   // For complex types like codeFinderRules
        public List<String> enumValues;  // For enum parameters
        public String[] enumLabels;      // Display labels for enum values

        // UI metadata from EditorDescription
        public String widget;           // "checkbox", "text", "select", "spin", "codeFinder", "path", "folder", "checkList"
        public String masterParam;      // Parameter that enables/disables this one
        public boolean enabledOnMasterSelected;  // True if enabled when master is selected
        public Integer minimum;         // For spin/integer inputs
        public Integer maximum;         // For spin/integer inputs

        // TextInputPart-specific
        public boolean allowEmpty;      // Text field can be empty
        public boolean password;        // Mask input as password
        public Integer textHeight;      // Multiline text area height (null = single line)

        // PathInputPart-specific
        public boolean forSaveAs;       // Save-as dialog (vs Open dialog)
        public String browseTitle;      // File/folder dialog title
        public String filterNames;      // File filter display names (e.g. "Documents")
        public String filterExtensions; // File filter extensions (e.g. "*.txt;*.odt")
        public boolean pathAllowEmpty;  // Path can be empty

        // CheckListPart-specific
        public List<ParamInfo> checkListEntries;  // Nested checkbox entries

        // Layout flags from AbstractPart
        public Boolean withLabel;       // Show label (null = default)
        public Boolean vertical;        // Vertical layout (null = default)

        public ParamInfo(String name, String type) {
            this.name = name;
            this.type = type;
        }
    }

    /**
     * Determine the serialization format used by a filter's parameters.
     * Returns "stringParameters" for #v1 key=value format, "xml-its" for XML ITS
     * rules (used by okf_xml and okf_html5), or "yaml" for YAML-based.
     */
    public String getSerializationFormat(String filterClass) {
        try {
            Class<?> clazz = Class.forName(filterClass);
            IFilter filter = (IFilter) clazz.getDeclaredConstructor().newInstance();
            IParameters params = filter.getParameters();
            if (params instanceof StringParameters) {
                return "stringParameters";
            }
            // Check if toString() produces XML (ITS parameters use DOM serialization)
            String serialized = params.toString();
            if (serialized != null &&
                    (serialized.startsWith("<?xml") || serialized.contains("<its:rules"))) {
                return "xml-its";
            }
            return "yaml";
        } catch (Exception e) {
            return "stringParameters"; // default assumption
        }
    }

    /**
     * Introspect a filter class to extract its parameter metadata.
     *
     * @param filterClass Fully-qualified filter class name
     * @return Map of parameter name to ParamInfo, or null if introspection fails
     */
    public Map<String, ParamInfo> introspect(String filterClass) {
        try {
            Class<?> clazz = Class.forName(filterClass);

            // Get the filter instance to access its parameters
            IFilter filter = (IFilter) clazz.getDeclaredConstructor().newInstance();
            IParameters params = filter.getParameters();

            if (params == null) {
                return null;
            }

            return introspectParameters(params);

        } catch (ClassNotFoundException e) {
            System.err.println("Filter class not found: " + filterClass);
            return null;
        } catch (Exception e) {
            System.err.println("Failed to introspect " + filterClass + ": " + e.getMessage());
            return null;
        }
    }

    /**
     * Introspect a Parameters class directly (for steps, which don't have an IFilter).
     *
     * @param paramsClass The Parameters class to introspect
     * @return Map of parameter name to ParamInfo, or null if introspection fails
     */
    public Map<String, ParamInfo> introspectParamsClass(Class<?> paramsClass) {
        try {
            Object obj = paramsClass.getDeclaredConstructor().newInstance();
            if (!(obj instanceof IParameters)) {
                return null;
            }
            return introspectParameters((IParameters) obj);
        } catch (Exception e) {
            System.err.println("Failed to introspect " + paramsClass.getName() + ": " + e.getMessage());
            return null;
        }
    }

    /**
     * Core introspection logic shared by filter and step paths.
     */
    private Map<String, ParamInfo> introspectParameters(IParameters params) {
        Map<String, ParamInfo> result = new LinkedHashMap<>();
        Class<?> paramsClass = params.getClass();

        if (params instanceof StringParameters) {
            introspectStringParameters(paramsClass, (StringParameters) params, result);
        } else {
            introspectByAccessors(paramsClass, params, result);
            introspectYamlConfig(params, result);
        }

        enrichWithParametersDescription(paramsClass, params, result);

        if (params instanceof IEditorDescriptionProvider) {
            enrichWithEditorDescription(paramsClass, (IEditorDescriptionProvider) params, result);
        }

        if (params instanceof ISimplifierRulesParameters) {
            addSimplifierRulesParams((ISimplifierRulesParameters) params, result);
        }

        return result;
    }
    
    /**
     * Enrich parameter info with UI metadata from createEditorDescription().
     *
     * Extracts widget types, master/slave relationships, enum values,
     * numeric constraints, layout flags, and part-specific metadata
     * (password, multiline, file browse filters, checklist entries, etc.).
     */
    private void enrichWithEditorDescription(Class<?> paramsClass, IEditorDescriptionProvider provider,
                                             Map<String, ParamInfo> result) {
        try {
            // First get ParametersDescription (required for createEditorDescription)
            Method descMethod = paramsClass.getMethod("getParametersDescription");
            ParametersDescription paramDesc = (ParametersDescription) descMethod.invoke(provider);

            if (paramDesc == null) {
                return;
            }

            // Now get EditorDescription
            EditorDescription editorDesc = provider.createEditorDescription(paramDesc);
            if (editorDesc == null) {
                return;
            }

            // Process each UI part
            for (Map.Entry<String, AbstractPart> entry : editorDesc.getDescriptors().entrySet()) {
                String paramName = entry.getKey();
                AbstractPart part = entry.getValue();

                ParamInfo info = result.get(paramName);
                if (info == null) {
                    // Parameter discovered via editor but not in our result set - try to add it
                    info = extractParamInfo(paramsClass, (IParameters) provider, paramName);
                    if (info != null) {
                        result.put(paramName, info);
                    } else {
                        continue;
                    }
                }

                // Extract widget type
                info.widget = mapPartToWidget(part);

                // Extract master/slave relationship
                AbstractPart masterPart = part.getMasterPart();
                if (masterPart != null) {
                    info.masterParam = masterPart.getName();
                    info.enabledOnMasterSelected = part.isEnabledOnSelection();
                }

                // Extract display name if not already set
                if (info.displayName == null && part.getDisplayName() != null) {
                    info.displayName = part.getDisplayName();
                }

                // Extract description if not already set
                if (info.description == null && part.getShortDescription() != null) {
                    info.description = part.getShortDescription();
                }

                // Layout flags from AbstractPart
                if (!part.isWithLabel()) {
                    info.withLabel = false;
                }
                if (part.isVertical()) {
                    info.vertical = true;
                }

                // Part-specific metadata
                if (part instanceof ListSelectionPart) {
                    ListSelectionPart listPart = (ListSelectionPart) part;
                    String[] choices = listPart.getChoicesValues();
                    if (choices != null && choices.length > 0) {
                        info.enumValues = Arrays.asList(choices);
                        info.enumLabels = listPart.getChoicesLabels();
                    }
                } else if (part instanceof SpinInputPart) {
                    SpinInputPart spinPart = (SpinInputPart) part;
                    int min = spinPart.getMinimumValue();
                    int max = spinPart.getMaximumValue();
                    if (min != Integer.MIN_VALUE) {
                        info.minimum = min;
                    }
                    if (max != Integer.MAX_VALUE) {
                        info.maximum = max;
                    }
                } else if (part instanceof TextInputPart) {
                    TextInputPart textPart = (TextInputPart) part;
                    if (textPart.isPassword()) {
                        info.password = true;
                    }
                    if (textPart.isAllowEmpty()) {
                        info.allowEmpty = true;
                    }
                    int height = textPart.getHeight();
                    if (height > 1) {
                        info.textHeight = height;
                    }
                } else if (part instanceof PathInputPart) {
                    PathInputPart pathPart = (PathInputPart) part;
                    if (pathPart.isForSaveAs()) {
                        info.forSaveAs = true;
                    }
                    if (pathPart.getBrowseTitle() != null && !pathPart.getBrowseTitle().isEmpty()) {
                        info.browseTitle = pathPart.getBrowseTitle();
                    }
                    String filterNames = pathPart.getFilterNames();
                    String filterExts = pathPart.getFilterExtensions();
                    if (filterNames != null && !filterNames.isEmpty()) {
                        info.filterNames = filterNames;
                    }
                    if (filterExts != null && !filterExts.isEmpty()) {
                        info.filterExtensions = filterExts;
                    }
                    if (pathPart.isAllowEmpty()) {
                        info.pathAllowEmpty = true;
                    }
                } else if (part instanceof FolderInputPart) {
                    FolderInputPart folderPart = (FolderInputPart) part;
                    if (folderPart.getBrowseTitle() != null && !folderPart.getBrowseTitle().isEmpty()) {
                        info.browseTitle = folderPart.getBrowseTitle();
                    }
                } else if (part instanceof CheckListPart) {
                    CheckListPart checkListPart = (CheckListPart) part;
                    Map<String, net.sf.okapi.common.ParameterDescriptor> entries = checkListPart.getEntries();
                    if (entries != null && !entries.isEmpty()) {
                        info.checkListEntries = new ArrayList<>();
                        for (Map.Entry<String, net.sf.okapi.common.ParameterDescriptor> clEntry : entries.entrySet()) {
                            ParamInfo entryInfo = new ParamInfo(clEntry.getKey(), "boolean");
                            net.sf.okapi.common.ParameterDescriptor pd = clEntry.getValue();
                            if (pd.getDisplayName() != null) {
                                entryInfo.displayName = pd.getDisplayName();
                            }
                            if (pd.getShortDescription() != null) {
                                entryInfo.description = pd.getShortDescription();
                            }
                            info.checkListEntries.add(entryInfo);
                        }
                    }
                }
            }
        } catch (NoSuchMethodException e) {
            // No getParametersDescription method
        } catch (Exception e) {
            // Ignore errors in editor description extraction
        }
    }
    
    /**
     * Map an AbstractPart subclass to a widget type string.
     */
    private String mapPartToWidget(AbstractPart part) {
        if (part instanceof CheckboxPart) {
            return "checkbox";
        } else if (part instanceof TextInputPart) {
            return "text";
        } else if (part instanceof ListSelectionPart) {
            ListSelectionPart listPart = (ListSelectionPart) part;
            return listPart.getListType() == ListSelectionPart.LISTTYPE_DROPDOWN ? "dropdown" : "select";
        } else if (part instanceof SpinInputPart) {
            return "spin";
        } else if (part instanceof CodeFinderPart) {
            return "codeFinder";
        } else if (part instanceof PathInputPart) {
            return "path";
        } else if (part instanceof FolderInputPart) {
            return "folder";
        } else if (part instanceof CheckListPart) {
            return "checkList";
        } else if (part instanceof SeparatorPart) {
            return "separator";
        } else if (part instanceof TextLabelPart) {
            return "label";
        }
        return null;
    }
    
    /**
     * Enrich parameter info with descriptions from getParametersDescription().
     */
    private void enrichWithParametersDescription(Class<?> paramsClass, IParameters params,
                                                  Map<String, ParamInfo> result) {
        try {
            Method descMethod = paramsClass.getMethod("getParametersDescription");
            ParametersDescription desc = (ParametersDescription) descMethod.invoke(params);
            
            if (desc != null) {
                for (Map.Entry<String, ParameterDescriptor> entry : desc.getDescriptors().entrySet()) {
                    String paramName = entry.getKey();
                    ParameterDescriptor pd = entry.getValue();
                    
                    ParamInfo info = result.get(paramName);
                    if (info != null) {
                        // Enrich existing parameter with description
                        if (pd.getDisplayName() != null) {
                            info.displayName = pd.getDisplayName();
                        }
                        if (pd.getShortDescription() != null) {
                            info.description = pd.getShortDescription();
                        }
                    } else {
                        // Parameter discovered via description but not found by field scan
                        // Try to determine its type and add it
                        info = extractParamInfo(paramsClass, params, paramName);
                        if (info != null) {
                            if (pd.getDisplayName() != null) {
                                info.displayName = pd.getDisplayName();
                            }
                            if (pd.getShortDescription() != null) {
                                info.description = pd.getShortDescription();
                            }
                            result.put(paramName, info);
                        }
                    }
                }
            }
        } catch (NoSuchMethodException e) {
            // No getParametersDescription method - that's OK
        } catch (Exception e) {
            // Ignore other errors
        }
    }
    
    /**
     * Add simplifier rules parameters from the ISimplifierRulesParameters interface.
     */
    private void addSimplifierRulesParams(ISimplifierRulesParameters params,
                                          Map<String, ParamInfo> result) {
        // simplifierRules - the main parameter from this interface
        String simplifierRulesKey = "simplifierRules";
        if (!result.containsKey(simplifierRulesKey)) {
            ParamInfo info = new ParamInfo(simplifierRulesKey, "string");
            info.displayName = "Simplifier Rules";
            info.description = "Simplifier Rules as defined in the Okapi Code Simplifier Rule Format";
            try {
                info.defaultValue = params.getSimplifierRules();
            } catch (Exception e) {
                // Ignore
            }
            result.put(simplifierRulesKey, info);
        }

        // Extract Optional<Boolean> properties via reflection (added in newer Okapi versions)
        extractOptionalBooleanParam(params, "getMoveLeadingAndTrailingCodesToSkeleton",
                "moveLeadingAndTrailingCodesToSkeleton",
                "Move Boundary Codes to Skeleton",
                "Move leading and trailing inline codes from segments to the skeleton",
                result);
        extractOptionalBooleanParam(params, "getMergeAdjacentCodes",
                "mergeAdjacentCodes",
                "Merge Adjacent Codes",
                "Merge consecutive inline codes into a single code",
                result);
    }

    /**
     * Extract an Optional&lt;Boolean&gt; parameter via reflection (for cross-version compatibility).
     */
    @SuppressWarnings("unchecked")
    private void extractOptionalBooleanParam(Object params, String methodName,
                                              String paramKey, String displayName, String description,
                                              Map<String, ParamInfo> result) {
        if (result.containsKey(paramKey)) return;
        try {
            Method m = params.getClass().getMethod(methodName);
            Object opt = m.invoke(params);
            if (opt instanceof Optional) {
                ParamInfo info = new ParamInfo(paramKey, "boolean");
                info.displayName = displayName;
                info.description = description;
                ((Optional<Boolean>) opt).ifPresent(v -> info.defaultValue = v);
                result.put(paramKey, info);
            }
        } catch (NoSuchMethodException e) {
            // Method not available in this Okapi version
        } catch (Exception e) {
            // Ignore
        }
    }

    /**
     * Introspect a StringParameters subclass by examining its constant fields
     * and getter/setter methods.
     */
    private void introspectStringParameters(Class<?> paramsClass, StringParameters params,
                                            Map<String, ParamInfo> result) {
        
        // Find all static final String fields in the class hierarchy (these are parameter names)
        // Include package-private and protected fields, not just private
        Set<String> paramNames = new LinkedHashSet<>();
        
        // Scan current class and all superclasses
        Class<?> currentClass = paramsClass;
        while (currentClass != null && currentClass != Object.class) {
            for (Field field : currentClass.getDeclaredFields()) {
                if (Modifier.isStatic(field.getModifiers()) &&
                    Modifier.isFinal(field.getModifiers()) &&
                    field.getType() == String.class) {
                    
                    field.setAccessible(true);
                    try {
                        String paramName = (String) field.get(null);
                        // Filter out non-parameter constants (usually all caps with underscores are params)
                        if (paramName != null && !paramName.isEmpty() && 
                            !paramName.contains(" ") && !paramName.startsWith("#")) {
                            paramNames.add(paramName);
                        }
                    } catch (Exception e) {
                        // Ignore
                    }
                }
            }
            currentClass = currentClass.getSuperclass();
        }
        
        // Also scan implemented interfaces for parameter name constants
        for (Class<?> iface : paramsClass.getInterfaces()) {
            for (Field field : iface.getDeclaredFields()) {
                // Interface fields are implicitly public static final
                if (field.getType() == String.class) {
                    try {
                        String paramName = (String) field.get(null);
                        if (paramName != null && !paramName.isEmpty() && 
                            !paramName.contains(" ") && !paramName.startsWith("#") &&
                            !paramName.endsWith("_DESC") && !paramName.endsWith("_NAME")) {
                            paramNames.add(paramName);
                        }
                    } catch (Exception e) {
                        // Ignore
                    }
                }
            }
        }

        // For each parameter name, find its getter to determine type and default
        for (String paramName : paramNames) {
            ParamInfo info = extractParamInfo(paramsClass, params, paramName);
            if (info != null) {
                result.put(paramName, info);
            }
        }

        // Also introspect public instance fields (used by table/plaintext filters)
        // These are fields like: public String fieldDelimiter, public boolean trimLeading
        introspectPublicInstanceFields(paramsClass, params, result);

        // Also check for complex objects like InlineCodeFinder
        introspectComplexFields(paramsClass, params, result);
    }

    /**
     * Extract parameter info by finding the getter method.
     */
    private ParamInfo extractParamInfo(Class<?> paramsClass, IParameters params, String paramName) {
        // Try common getter patterns
        String[] getterPrefixes = {"get", "is"};
        String camelName = toCamelCase(paramName);
        
        for (String prefix : getterPrefixes) {
            String methodName = prefix + Character.toUpperCase(camelName.charAt(0)) + camelName.substring(1);
            
            try {
                Method getter = paramsClass.getMethod(methodName);
                Class<?> returnType = getter.getReturnType();
                
                ParamInfo info = new ParamInfo(paramName, mapJavaType(returnType));
                
                // Get default value
                try {
                    // Reset to defaults and get value
                    params.reset();
                    info.defaultValue = getter.invoke(params);
                } catch (Exception e) {
                    // Default value unknown
                }
                
                // Extract description from Javadoc (if available via annotation)
                info.description = extractDescription(getter);
                
                return info;
                
            } catch (NoSuchMethodException e) {
                // Try next pattern
            }
        }

        // If no getter found, try to determine type from IParameters interface
        try {
            // Try getBoolean
            params.reset();
            boolean boolVal = params.getBoolean(paramName);
            ParamInfo info = new ParamInfo(paramName, "boolean");
            info.defaultValue = boolVal;
            return info;
        } catch (Exception e) {
            // Not a boolean
        }

        try {
            // Try getString
            params.reset();
            String strVal = params.getString(paramName);
            if (strVal != null) {
                ParamInfo info = new ParamInfo(paramName, "string");
                info.defaultValue = strVal;
                return info;
            }
        } catch (Exception e) {
            // Not a string
        }

        try {
            // Try getInteger
            params.reset();
            int intVal = params.getInteger(paramName);
            ParamInfo info = new ParamInfo(paramName, "integer");
            info.defaultValue = intVal;
            return info;
        } catch (Exception e) {
            // Not an integer
        }

        // Unknown type - default to string
        ParamInfo info = new ParamInfo(paramName, "string");
        return info;
    }

    /**
     * Introspect public instance fields as parameters.
     * Many Okapi filters (table, plaintext) use public fields directly for configuration.
     */
    private void introspectPublicInstanceFields(Class<?> paramsClass, IParameters params,
                                                 Map<String, ParamInfo> result) {
        // Reset to get default values
        try {
            params.reset();
        } catch (Exception e) {
            // Ignore reset failures
        }
        
        // Internal fields that are not user-configurable parameters
        Set<String> internalFields = new HashSet<>(java.util.Arrays.asList(
            "data", "path", "parametersClass", "defParametersClass", 
            "codeFinder", "logger", "LOGGER", "parentFilter"
        ));
        
        // Scan current class and all superclasses for public instance fields
        Class<?> currentClass = paramsClass;
        while (currentClass != null && currentClass != Object.class) {
            for (Field field : currentClass.getDeclaredFields()) {
                // Skip static fields, we want instance fields
                if (Modifier.isStatic(field.getModifiers())) continue;
                
                // Skip already processed parameters
                String fieldName = field.getName();
                if (result.containsKey(fieldName)) continue;
                
                // Skip internal fields
                if (internalFields.contains(fieldName)) continue;
                
                // Skip non-configurable types
                Class<?> fieldType = field.getType();
                if (!isConfigurableType(fieldType)) continue;
                
                field.setAccessible(true);
                try {
                    ParamInfo info = new ParamInfo(fieldName, mapJavaType(fieldType));
                    
                    // Get default value
                    Object defaultVal = field.get(params);
                    if (defaultVal != null) {
                        // Handle enum fields
                        if (fieldType.isEnum()) {
                            info.defaultValue = defaultVal.toString();
                            info.enumValues = new java.util.ArrayList<>();
                            for (Object c : fieldType.getEnumConstants()) {
                                info.enumValues.add(c.toString());
                            }
                        } else {
                            info.defaultValue = defaultVal;
                        }
                    }
                    
                    result.put(fieldName, info);
                } catch (Exception e) {
                    // Skip fields that can't be accessed
                }
            }
            currentClass = currentClass.getSuperclass();
        }
    }
    
    /**
     * Check if a field type is a configurable parameter type.
     */
    private boolean isConfigurableType(Class<?> type) {
        return type == String.class ||
               type == boolean.class || type == Boolean.class ||
               type == int.class || type == Integer.class ||
               type == long.class || type == Long.class ||
               type == double.class || type == Double.class ||
               type == float.class || type == Float.class ||
               type.isEnum();
    }

    /**
     * Look for complex fields like InlineCodeFinder.
     */
    private void introspectComplexFields(Class<?> paramsClass, IParameters params,
                                         Map<String, ParamInfo> result) {
        
        for (Field field : paramsClass.getDeclaredFields()) {
            if (Modifier.isStatic(field.getModifiers())) continue;
            
            field.setAccessible(true);
            
            try {
                if (field.getType() == InlineCodeFinder.class) {
                    // This is handled via codeFinderRules string parameter
                    // Mark the corresponding parameter as needing transformation
                    ParamInfo existing = result.get("codeFinderRules");
                    if (existing != null) {
                        existing.okapiFormat = "inlineCodeFinder";
                        existing.type = "object";  // Will be transformed to clean object format
                    } else {
                        // Some StringParameters filters (e.g. DTD) hold an
                        // InlineCodeFinder and serialize it via
                        // setGroup("codeFinderRules", ...) / getGroup(...) but
                        // declare no String constant for it, so the constant
                        // scan above never surfaces the parameter. Create it
                        // here so the rich code-finder editor is exposed, on a
                        // par with filters that do declare the constant.
                        ParamInfo created = new ParamInfo("codeFinderRules", "object");
                        created.okapiFormat = "inlineCodeFinder";
                        result.put("codeFinderRules", created);
                    }
                }
            } catch (Exception e) {
                // Ignore
            }
        }
    }


	// Properties that come from the AbstractMarkupParameters wrapper, not the YAML config
	private static final Set<String> YAML_INTERNAL_PROPS = new HashSet<>(Arrays.asList(
		"taggedConfig", "editorTitle", "path", "data"
	));

	/**
	 * Introspect YAML-based filter configurations (AbstractMarkupParameters / TaggedFilterConfiguration).
	 * Parses the default YAML config to extract top-level properties with their types.
	 */
	@SuppressWarnings("unchecked")
	private void introspectYamlConfig(IParameters params, Map<String, ParamInfo> result) {
		try {
			// Reset to get defaults, then serialize to YAML string
			params.reset();
			String yamlStr = params.toString();
			if (yamlStr == null || yamlStr.isEmpty()) return;

			// Use a resolver that matches YAML 1.2 boolean rules (only true/false,
			// not yes/no/on/off). params.toString() uses snakeyaml-engine (YAML 1.2),
			// so unquoted "no"/"yes" are strings, not booleans.
			Yaml yaml = new Yaml(new org.yaml.snakeyaml.constructor.Constructor(
					new org.yaml.snakeyaml.LoaderOptions()),
				new org.yaml.snakeyaml.representer.Representer(
					new org.yaml.snakeyaml.DumperOptions()),
				new org.yaml.snakeyaml.DumperOptions(),
				new Yaml12BoolResolver());
			Object parsed = yaml.load(yamlStr);
			if (!(parsed instanceof Map)) return;

			Map<String, Object> config = (Map<String, Object>) parsed;

			for (Map.Entry<String, Object> entry : config.entrySet()) {
				String key = entry.getKey();
				Object value = entry.getValue();

				// Skip properties already discovered by accessor introspection
				if (result.containsKey(key)) continue;
				// Skip internal wrapper properties
				if (YAML_INTERNAL_PROPS.contains(key)) continue;

				if (value instanceof Boolean) {
					ParamInfo info = new ParamInfo(key, "boolean");
					info.defaultValue = value;
					result.put(key, info);
				} else if (value instanceof Integer || value instanceof Long) {
					ParamInfo info = new ParamInfo(key, "integer");
					info.defaultValue = value;
					result.put(key, info);
				} else if (value instanceof Number) {
					ParamInfo info = new ParamInfo(key, "number");
					info.defaultValue = value;
					result.put(key, info);
				} else if (value instanceof String) {
					ParamInfo info = new ParamInfo(key, "string");
					info.defaultValue = value;
					result.put(key, info);
				} else if (value instanceof Map) {
					// Complex map properties like "elements" and "attributes"
					ParamInfo info = new ParamInfo(key, "object");
					info.defaultValue = value;
					if ("elements".equals(key)) {
						info.okapiFormat = "elementRules";
					} else if ("attributes".equals(key)) {
						info.okapiFormat = "attributeRules";
					}
					result.put(key, info);
				} else if (value instanceof List) {
					ParamInfo info = new ParamInfo(key, "array");
					info.defaultValue = value;
					result.put(key, info);
				}
			}
		} catch (Exception e) {
			// YAML parsing failed - skip silently
			System.err.println("YAML config introspection failed: " + e.getMessage());
		}
	}

    /**
     * Introspect by examining getter/setter methods.
     */
    private void introspectByAccessors(Class<?> paramsClass, IParameters params,
                                       Map<String, ParamInfo> result) {
        
        Set<String> processedNames = new HashSet<>();
        
        for (Method method : paramsClass.getMethods()) {
            String name = method.getName();
            
            // Skip Object methods and internal methods
            if (method.getDeclaringClass() == Object.class) continue;
            if (name.equals("getClass")) continue;
            
            // Find getters (getXxx or isXxx with no parameters)
            if (method.getParameterCount() == 0 && !method.getReturnType().equals(void.class)) {
                String paramName = null;
                
                if (name.startsWith("get") && name.length() > 3) {
                    paramName = Character.toLowerCase(name.charAt(3)) + name.substring(4);
                } else if (name.startsWith("is") && name.length() > 2 &&
                           method.getReturnType() == boolean.class) {
                    paramName = Character.toLowerCase(name.charAt(2)) + name.substring(3);
                }
                
                if (paramName != null && !processedNames.contains(paramName)) {
                    // Check if there's a corresponding setter
                    String setterName = "set" + Character.toUpperCase(paramName.charAt(0)) + paramName.substring(1);
                    try {
                        paramsClass.getMethod(setterName, method.getReturnType());
                        // Has setter - this is a configurable parameter
                        
                        ParamInfo info = new ParamInfo(paramName, mapJavaType(method.getReturnType()));
                        
                        try {
                            params.reset();
                            info.defaultValue = method.invoke(params);
                        } catch (Exception e) {
                            // Default unknown
                        }
                        
                        info.description = extractDescription(method);
                        result.put(paramName, info);
                        processedNames.add(paramName);
                        
                    } catch (NoSuchMethodException e) {
                        // No setter - probably not a configurable parameter
                    }
                }
            }
        }
    }

    /**
     * Map Java types to JSON Schema types.
     */
    private String mapJavaType(Class<?> javaType) {
        if (javaType == boolean.class || javaType == Boolean.class) {
            return "boolean";
        } else if (javaType == int.class || javaType == Integer.class ||
                   javaType == long.class || javaType == Long.class) {
            return "integer";
        } else if (javaType == float.class || javaType == Float.class ||
                   javaType == double.class || javaType == Double.class) {
            return "number";
        } else if (javaType == String.class) {
            return "string";
        } else if (javaType.isArray() || Collection.class.isAssignableFrom(javaType)) {
            return "array";
        } else if (javaType.isEnum()) {
            return "string";  // Enums become strings with enum constraint
        } else if (javaType == Optional.class) {
            // Optional<T> — we can't resolve T at runtime via Class alone,
            // but in Okapi it's always Optional<Boolean>
            return "boolean";
        } else {
            return "object";
        }
    }

    /**
     * Convert underscore/lowercase name to camelCase.
     */
    private String toCamelCase(String name) {
        // Most Okapi parameter names are already camelCase
        return name;
    }

    /**
     * Extract description from method annotations or Javadoc.
     */
    private String extractDescription(Method method) {
        // In the future, we could use annotation processors or parse Javadoc
        // For now, return null - descriptions will come from editorHints
        return null;
    }

    /**
     * Custom SnakeYAML Resolver that uses YAML 1.2 boolean rules.
     * In YAML 1.2 only "true" and "false" are booleans, not "yes"/"no"/"on"/"off".
     * This is needed because params.toString() uses snakeyaml-engine (YAML 1.2)
     * where unquoted "no"/"yes" are plain strings.
     */
    private static class Yaml12BoolResolver extends Resolver {
        @Override
        protected void addImplicitResolvers() {
            super.addImplicitResolvers();
            // Replace the BOOL implicit resolver with one that only matches true/false
            // (YAML 1.2 core schema). The parent already added the YAML 1.1 pattern.
            // We re-add BOOL with a stricter pattern that overrides by priority.
        }

        @Override
        public org.yaml.snakeyaml.nodes.Tag resolve(org.yaml.snakeyaml.nodes.NodeId kind,
                String value, boolean implicit) {
            if (implicit && kind == org.yaml.snakeyaml.nodes.NodeId.scalar && value != null) {
                String lower = value.toLowerCase();
                // YAML 1.1 treats yes/no/on/off/y/n as booleans; YAML 1.2 does not
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
