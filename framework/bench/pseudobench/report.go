package main

import (
	"encoding/json"
	"html/template"
	"os"
	"path/filepath"
)

const htmlTemplate = `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>PseudoBench Results</title>
    <script src="https://cdn.jsdelivr.net/npm/chart.js"></script>
    <script src="https://cdn.jsdelivr.net/npm/chartjs-chart-error-bars"></script>
    <style>
        * { margin: 0; padding: 0; box-sizing: border-box; }
        body {
            font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, Helvetica, Arial, sans-serif;
            padding: 2rem;
            max-width: 1200px;
            margin: 0 auto;
            color: #1a1a1a;
            background: #fff;
        }
        h1 { font-size: 1.5rem; margin-bottom: 0.5rem; }
        h2 { font-size: 1.1rem; margin-bottom: 0.75rem; }
        .meta { color: #666; font-size: 0.85rem; margin-bottom: 2rem; line-height: 1.6; }
        .charts { display: grid; grid-template-columns: 1fr 1fr; gap: 2rem; margin-bottom: 2rem; }
        .chart-container {
            background: #fafafa;
            border: 1px solid #e0e0e0;
            border-radius: 8px;
            padding: 1.25rem;
        }
        .chart-container h2 { font-size: 1rem; margin-bottom: 0.75rem; color: #333; }
        .full-width { grid-column: 1 / -1; }
        .chart-container.tall { min-height: 300px; }
        canvas { width: 100% !important; }
        table { width: 100%; border-collapse: collapse; font-size: 0.85rem; margin-top: 0.5rem; }
        th, td { padding: 0.6rem 0.75rem; text-align: left; border-bottom: 1px solid #e0e0e0; }
        th { background: #f5f5f5; font-weight: 600; color: #333; }
        tr:hover { background: #f9f9f9; }
        td.numeric { text-align: right; font-variant-numeric: tabular-nums; }
        th.numeric { text-align: right; }
        .engine-label { font-weight: 600; }
        .section { margin-bottom: 2rem; }
        .legend-row { display: flex; flex-wrap: wrap; gap: 1rem; margin-top: 0.5rem; font-size: 0.8rem; color: #555; }
        .legend-item { display: flex; align-items: center; gap: 0.3rem; }
        .legend-swatch { width: 12px; height: 12px; border-radius: 2px; display: inline-block; }
        @media (max-width: 768px) { .charts { grid-template-columns: 1fr; } }
    </style>
</head>
<body>
    <h1>PseudoBench Results</h1>
    <div class="meta">
        {{.Metadata.Timestamp}} &middot; {{.Metadata.Platform}} &middot; {{.Metadata.CPUModel}} ({{.Metadata.CPUCores}} cores) &middot; {{printf "%.0f" .Metadata.MemoryGB}} GB RAM &middot; {{.Metadata.GoVersion}}
    </div>

    <div class="charts">
        <div class="chart-container">
            <h2>Wall Time (ms) &mdash; median with P5&ndash;P95 range</h2>
            <canvas id="timeChart"></canvas>
        </div>
        <div class="chart-container">
            <h2>Peak RSS (KB)</h2>
            <canvas id="rssChart"></canvas>
        </div>

        <div class="chart-container full-width tall">
            <h2>File Processing Timeline</h2>
            <canvas id="timelineChart"></canvas>
            <div class="legend-row" id="timelineLegend"></div>
        </div>
    </div>

    <div class="section">
        <h2>Per-File Comparison</h2>
        <table id="fileComparisonTable">
            <thead><tr id="fileComparisonHead"></tr></thead>
            <tbody id="fileComparisonBody"></tbody>
        </table>
    </div>

    <div class="section">
        <h2>Detailed Results</h2>
        <table>
            <thead>
                <tr>
                    <th>Engine</th>
                    <th>Version</th>
                    <th>Files</th>
                    <th class="numeric">Wall Time (ms)</th>
                    <th class="numeric">Peak RSS (KB)</th>
                    <th class="numeric">Daemon RSS (KB)</th>
                </tr>
            </thead>
            <tbody>
                {{range .Experiments}}
                <tr>
                    <td class="engine-label">{{.Engine}}</td>
                    <td>{{.Version}}</td>
                    <td>{{successCount .FileTimings}}/{{len .FileTimings}}</td>
                    <td class="numeric">{{printf "%.0f" .WallTimeMs.Median}} &plusmn; {{printf "%.0f" .WallTimeMs.Stddev}}</td>
                    <td class="numeric">{{printf "%.0f" .PeakRssKB.Max}}</td>
                    <td class="numeric">{{if .DaemonRssKB}}{{printf "%.0f" .DaemonRssKB.Max}}{{else}}&mdash;{{end}}</td>
                </tr>
                {{end}}
            </tbody>
        </table>
    </div>

    <script>
    var experiments = {{.DataJSON}};

    var ENGINE_COLORS = {
        "kapi-native":        { bg: "rgba(33,150,243,0.7)",  border: "#2196F3" },
        "kapi-bridge":        { bg: "rgba(255,152,0,0.7)",   border: "#FF9800" },
        "kapi-bridge-daemon": { bg: "rgba(76,175,80,0.7)",   border: "#4CAF50" },
        "okapi":              { bg: "rgba(156,39,176,0.7)",  border: "#9C27B0" },
    };

    var FORMAT_COLORS = {
        "openxml":     "#4CAF50",
        "html":        "#2196F3",
        "xliff":       "#FF9800",
        "po":          "#9C27B0",
        "yaml":        "#F44336",
        "json":        "#00BCD4",
        "xml":         "#795548",
        "properties":  "#607D8B",
        "srt":         "#E91E63",
    };

    function colorFor(engine) {
        return ENGINE_COLORS[engine] || { bg: "rgba(158,158,158,0.7)", border: "#9E9E9E" };
    }

    function findTiming(timings, name) {
        for (var i = 0; i < timings.length; i++) {
            if (timings[i].name === name) return timings[i];
        }
        return null;
    }

    // ---------------------------------------------------------------------------
    // 1. Wall Time chart
    // ---------------------------------------------------------------------------
    new Chart(document.getElementById("timeChart"), {
        type: "barWithErrorBars",
        data: {
            labels: experiments.map(function(e) { return e.engine; }),
            datasets: [{
                label: "Wall Time (median ms)",
                data: experiments.map(function(e) {
                    return { y: e.wallTimeMs.median, yMin: e.wallTimeMs.p5, yMax: e.wallTimeMs.p95 };
                }),
                backgroundColor: experiments.map(function(e) { return colorFor(e.engine).bg; }),
                borderColor: experiments.map(function(e) { return colorFor(e.engine).border; }),
                borderWidth: 1,
                errorBarLineWidth: 1.5,
                errorBarWhiskerLineWidth: 1.5,
                errorBarColor: "#333",
                errorBarWhiskerColor: "#333",
            }],
        },
        options: {
            responsive: true,
            plugins: {
                legend: { display: false },
                tooltip: {
                    callbacks: {
                        label: function(ctx) {
                            var e = experiments[ctx.dataIndex];
                            return "Median: " + e.wallTimeMs.median.toFixed(0)
                                + " ms  (P5: " + e.wallTimeMs.p5.toFixed(0)
                                + ", P95: " + e.wallTimeMs.p95.toFixed(0) + ")";
                        }
                    }
                }
            },
            scales: {
                x: { grid: { display: false } },
                y: { beginAtZero: true, title: { display: true, text: "ms" } }
            }
        }
    });

    // ---------------------------------------------------------------------------
    // 2. Peak RSS chart
    // ---------------------------------------------------------------------------
    var hasDaemon = experiments.some(function(e) { return e.daemonRssKB; });

    var rssDatasets = [{
        label: "Peak RSS (kapi)",
        data: experiments.map(function(e) { return e.peakRssKB.max; }),
        backgroundColor: experiments.map(function(e) { return colorFor(e.engine).bg; }),
        borderColor: experiments.map(function(e) { return colorFor(e.engine).border; }),
        borderWidth: 1,
    }];

    if (hasDaemon) {
        rssDatasets.push({
            label: "Daemon JVM RSS",
            data: experiments.map(function(e) { return e.daemonRssKB ? e.daemonRssKB.max : 0; }),
            backgroundColor: experiments.map(function(e) {
                return e.daemonRssKB ? "rgba(76,175,80,0.35)" : "rgba(0,0,0,0)";
            }),
            borderColor: experiments.map(function(e) {
                return e.daemonRssKB ? "#388E3C" : "rgba(0,0,0,0)";
            }),
            borderWidth: 1,
        });
    }

    new Chart(document.getElementById("rssChart"), {
        type: "bar",
        data: {
            labels: experiments.map(function(e) { return e.engine; }),
            datasets: rssDatasets,
        },
        options: {
            responsive: true,
            plugins: {
                legend: { display: hasDaemon, position: "bottom" },
                tooltip: {
                    callbacks: {
                        label: function(ctx) {
                            return ctx.dataset.label + ": " + (ctx.parsed.y || 0).toFixed(0) + " KB";
                        }
                    }
                }
            },
            scales: {
                x: { grid: { display: false }, stacked: hasDaemon },
                y: { beginAtZero: true, stacked: hasDaemon, title: { display: true, text: "KB" } }
            }
        }
    });

    // ---------------------------------------------------------------------------
    // 3. Timeline chart
    // ---------------------------------------------------------------------------
    var allTimings = [];
    var fileNameSet = {};
    var fileNames = [];
    experiments.forEach(function(e) {
        (e.fileTimings || []).forEach(function(ft) {
            allTimings.push(ft);
            if (!fileNameSet[ft.name]) {
                fileNameSet[ft.name] = true;
                fileNames.push(ft.name);
            }
        });
    });
    var engineLabels = experiments.map(function(e) { return e.engine; });

    var timelineDatasets = fileNames.map(function(fileName) {
        var format = "";
        for (var i = 0; i < experiments.length; i++) {
            var ft = findTiming(experiments[i].fileTimings || [], fileName);
            if (ft) { format = ft.format; break; }
        }
        var color = FORMAT_COLORS[format] || "#999";
        return {
            label: fileName,
            data: experiments.map(function(exp) {
                var ft = findTiming(exp.fileTimings || [], fileName);
                return ft ? [ft.startMs, ft.endMs] : null;
            }),
            backgroundColor: color + "CC",
            borderColor: color,
            borderWidth: 0.5,
            borderSkipped: false,
        };
    });

    new Chart(document.getElementById("timelineChart"), {
        type: "bar",
        data: { labels: engineLabels, datasets: timelineDatasets },
        options: {
            indexAxis: "y",
            responsive: true,
            plugins: {
                legend: { display: false },
                tooltip: {
                    callbacks: {
                        title: function(items) { return items[0] ? items[0].dataset.label : ""; },
                        label: function(ctx) {
                            var exp = experiments[ctx.dataIndex];
                            var ft = findTiming(exp.fileTimings || [], ctx.dataset.label);
                            if (!ft) return "";
                            return [
                                ft.format + " \u00b7 " + (ft.sizeBytes / 1024).toFixed(0) + " KB",
                                "Wall: " + ft.wallMs.toFixed(0) + " ms",
                                "RSS: " + (ft.peakRssKB / 1024).toFixed(0) + " MB",
                                "CPU: " + (ft.userCpuMs + ft.sysCpuMs).toFixed(0) + " ms",
                            ];
                        }
                    }
                }
            },
            scales: {
                x: { stacked: true, title: { display: true, text: "Time (ms)" }, beginAtZero: true },
                y: { stacked: true }
            }
        }
    });

    // Build format legend
    (function() {
        var usedFormats = {};
        allTimings.forEach(function(ft) { usedFormats[ft.format] = true; });
        var sortedFormats = Object.keys(usedFormats).sort();
        var legendEl = document.getElementById("timelineLegend");
        sortedFormats.forEach(function(fmt) {
            var color = FORMAT_COLORS[fmt] || "#999";
            var item = document.createElement("span");
            item.className = "legend-item";
            var swatch = document.createElement("span");
            swatch.className = "legend-swatch";
            swatch.style.background = color;
            item.appendChild(swatch);
            item.appendChild(document.createTextNode(fmt));
            legendEl.appendChild(item);
        });
    })();

    // ---------------------------------------------------------------------------
    // 4. Per-File Comparison table
    // ---------------------------------------------------------------------------
    (function() {
        var headRow = document.getElementById("fileComparisonHead");
        var tbody = document.getElementById("fileComparisonBody");

        ["File", "Format", "Size"].forEach(function(text) {
            var th = document.createElement("th");
            th.textContent = text;
            if (text === "Size") th.className = "numeric";
            headRow.appendChild(th);
        });
        experiments.forEach(function(exp) {
            var th = document.createElement("th");
            th.textContent = exp.engine + " (ms)";
            th.className = "numeric";
            headRow.appendChild(th);
        });

        fileNames.forEach(function(fileName) {
            var tr = document.createElement("tr");

            var format = "", sizeBytes = 0;
            for (var i = 0; i < experiments.length; i++) {
                var ft = findTiming(experiments[i].fileTimings || [], fileName);
                if (ft) { format = ft.format; sizeBytes = ft.sizeBytes; break; }
            }

            var tdName = document.createElement("td");
            tdName.textContent = fileName;
            tr.appendChild(tdName);

            var tdFmt = document.createElement("td");
            tdFmt.textContent = format;
            tr.appendChild(tdFmt);

            var tdSize = document.createElement("td");
            tdSize.className = "numeric";
            tdSize.textContent = (sizeBytes / 1024).toFixed(0) + " KB";
            tr.appendChild(tdSize);

            experiments.forEach(function(exp) {
                var td = document.createElement("td");
                td.className = "numeric";
                var ft = findTiming(exp.fileTimings || [], fileName);
                td.textContent = ft ? ft.wallMs.toFixed(0) : "\u2014";
                tr.appendChild(td);
            });

            tbody.appendChild(tr);
        });
    })();
    </script>
</body>
</html>`

type templateData struct {
	Metadata    Metadata
	Experiments []ExperimentResult
	DataJSON    template.JS
}

func generateHTMLReport(report *Report, outputPath string) error {
	if err := os.MkdirAll(filepath.Dir(outputPath), 0o755); err != nil {
		return err
	}

	dataBytes, err := json.Marshal(report.Experiments)
	if err != nil {
		return err
	}

	funcMap := template.FuncMap{
		"successCount": func(timings []FileTiming) int {
			count := 0
			for _, t := range timings {
				if t.Success {
					count++
				}
			}
			return count
		},
	}

	td := templateData{
		Metadata:    report.Metadata,
		Experiments: report.Experiments,
		DataJSON:    template.JS(dataBytes),
	}

	tmpl, err := template.New("report").Funcs(funcMap).Parse(htmlTemplate)
	if err != nil {
		return err
	}

	f, err := os.Create(outputPath)
	if err != nil {
		return err
	}
	defer f.Close()

	return tmpl.Execute(f, td)
}
