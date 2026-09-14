// OkapiSeg: parity oracle. Reads a TSV corpus (locale<TAB>text) on argv[1],
// segments each line with the REAL Okapi SRXSegmenter using the same
// defaultSegmentation.srx, and prints one JSON object per line:
//   {"locale":"en","text":"...","segments":["First.","Second."]}
// Segment texts are whitespace-trimmed so comparison ignores trim-policy.
// Compile/run via scripts/srx-parity/gen-golden.sh.
import net.sf.okapi.common.ISegmenter;
import net.sf.okapi.common.LocaleId;
import net.sf.okapi.common.Range;
import net.sf.okapi.lib.segmentation.SRXDocument;

import java.nio.file.*;
import java.util.*;

public class OkapiSeg {
    public static void main(String[] args) throws Exception {
        String srxPath = args[0];
        String corpus = args[1];
        SRXDocument doc = new SRXDocument();
        doc.loadRules(srxPath);
        Map<String, ISegmenter> byLoc = new HashMap<>();
        StringBuilder out = new StringBuilder();
        for (String line : Files.readAllLines(Paths.get(corpus))) {
            if (line.isEmpty() || line.startsWith("#")) continue;
            int tab = line.indexOf('\t');
            String loc = line.substring(0, tab);
            String text = line.substring(tab + 1);
            ISegmenter seg = byLoc.computeIfAbsent(loc,
                k -> doc.compileLanguageRules(LocaleId.fromString(k), null));
            seg.computeSegments(text);
            List<Range> ranges = seg.getRanges();
            List<String> segs = new ArrayList<>();
            for (Range r : ranges) {
                String s = text.substring(r.start, r.end).trim();
                if (!s.isEmpty()) segs.add(s);
            }
            out.append("{\"locale\":").append(js(loc))
               .append(",\"text\":").append(js(text))
               .append(",\"segments\":[");
            for (int i = 0; i < segs.size(); i++) {
                if (i > 0) out.append(',');
                out.append(js(segs.get(i)));
            }
            out.append("]}\n");
        }
        System.out.print(out);
    }

    static String js(String s) {
        StringBuilder b = new StringBuilder("\"");
        for (int i = 0; i < s.length(); i++) {
            char c = s.charAt(i);
            switch (c) {
                case '"': b.append("\\\""); break;
                case '\\': b.append("\\\\"); break;
                case '\n': b.append("\\n"); break;
                case '\r': b.append("\\r"); break;
                case '\t': b.append("\\t"); break;
                default:
                    if (c < 0x20) b.append(String.format("\\u%04x", (int) c));
                    else b.append(c);
            }
        }
        return b.append('"').toString();
    }
}
