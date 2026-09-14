// Prints the comments javac's own tokenizer finds in each file named on the
// command line, one per line: the path, the start and end offsets in UTF-16 code
// units, and LINE, BLOCK or JAVADOC. java-goldens.py turns them into goldens.
//
// The tokenizer is internal to the JDK, so the program runs with
// --add-exports for jdk.compiler/com.sun.tools.javac.parser and
// jdk.compiler/com.sun.tools.javac.util.

import com.sun.tools.javac.parser.JavaTokenizer;
import com.sun.tools.javac.parser.ScannerFactory;
import com.sun.tools.javac.parser.Tokens;
import com.sun.tools.javac.util.Context;
import com.sun.tools.javac.util.Log;
import java.nio.file.Files;
import java.nio.file.Path;

public class JavaComments {
    public static void main(String[] args) throws Exception {
        Context context = new Context();
        Log.instance(context);
        ScannerFactory factory = ScannerFactory.instance(context);
        for (String path : args) {
            char[] chars = Files.readString(Path.of(path)).toCharArray();
            JavaTokenizer tokenizer = new JavaTokenizer(factory, chars, chars.length) {
                @Override
                protected Tokens.Comment processComment(int pos, int endPos, Tokens.Comment.CommentStyle style) {
                    System.out.println(path + "\t" + pos + "\t" + endPos + "\t" + style);
                    return super.processComment(pos, endPos, style);
                }
            };
            while (tokenizer.readToken().kind != Tokens.TokenKind.EOF) {
            }
        }
    }
}
