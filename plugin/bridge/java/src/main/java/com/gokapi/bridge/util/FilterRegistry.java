package com.gokapi.bridge.util;

import com.gokapi.bridge.model.FilterInfo;
import net.sf.okapi.common.filters.IFilter;

import java.util.*;

/**
 * Registry of known Okapi filter classes.
 * Maps fully-qualified class names to metadata and provides filter instantiation.
 */
public class FilterRegistry {

    private static final Map<String, FilterInfo> FILTERS = new LinkedHashMap<>();

    static {
        register(new FilterInfo(
                "net.sf.okapi.filters.openxml.OpenXMLFilter",
                "openxml",
                "Microsoft Office (OpenXML)",
                Arrays.asList(
                        "application/vnd.openxmlformats-officedocument.wordprocessingml.document",
                        "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet",
                        "application/vnd.openxmlformats-officedocument.presentationml.presentation"
                ),
                Arrays.asList(".docx", ".xlsx", ".pptx")
        ));

        register(new FilterInfo(
                "net.sf.okapi.filters.html.HtmlFilter",
                "html",
                "HTML",
                Collections.singletonList("text/html"),
                Arrays.asList(".html", ".htm")
        ));

        register(new FilterInfo(
                "net.sf.okapi.filters.xliff.XLIFFFilter",
                "xliff",
                "XLIFF",
                Collections.singletonList("application/xliff+xml"),
                Arrays.asList(".xlf", ".xliff")
        ));

        register(new FilterInfo(
                "net.sf.okapi.filters.its.xml.ITSFilter",
                "xml",
                "XML (ITS)",
                Arrays.asList("text/xml", "application/xml"),
                Collections.singletonList(".xml")
        ));

        register(new FilterInfo(
                "net.sf.okapi.filters.properties.PropertiesFilter",
                "properties",
                "Java Properties",
                Collections.emptyList(),
                Collections.singletonList(".properties")
        ));

        register(new FilterInfo(
                "net.sf.okapi.filters.po.POFilter",
                "po",
                "PO (Gettext)",
                Collections.singletonList("text/x-po"),
                Arrays.asList(".po", ".pot")
        ));
    }

    private static void register(FilterInfo info) {
        FILTERS.put(info.getFilterClass(), info);
    }

    /**
     * Get metadata for a filter class.
     *
     * @param filterClass fully-qualified Java class name
     * @return FilterInfo or null if not found
     */
    public static FilterInfo getFilterInfo(String filterClass) {
        return FILTERS.get(filterClass);
    }

    /**
     * Create a new instance of the specified filter.
     *
     * @param filterClass fully-qualified Java class name
     * @return new IFilter instance or null
     */
    public static IFilter createFilter(String filterClass) {
        try {
            Class<?> clazz = Class.forName(filterClass);
            Object instance = clazz.getDeclaredConstructor().newInstance();
            if (instance instanceof IFilter) {
                return (IFilter) instance;
            }
            return null;
        } catch (Exception e) {
            System.err.println("[bridge] Failed to instantiate filter " + filterClass + ": " + e.getMessage());
            return null;
        }
    }

    /**
     * List all registered filters.
     */
    public static List<FilterInfo> listFilters() {
        return new ArrayList<>(FILTERS.values());
    }
}
