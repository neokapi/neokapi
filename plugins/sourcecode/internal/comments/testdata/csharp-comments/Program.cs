// Prints the comments Roslyn finds in each file named on the command line, one
// per line: the path, the start and end offsets in UTF-16 code units, and line or
// block. Roslyn reads a run of /// lines as one documentation trivia; each line
// is printed as a comment of its own, from its /// to the end of the line.
// Roslyn does not lex a region its preprocessor sets aside, such as the body of
// an #if whose symbol is undefined, so the program lexes that region's text with
// Roslyn again. csharp-goldens.py turns the output into goldens.

using System;
using System.IO;
using Microsoft.CodeAnalysis;
using Microsoft.CodeAnalysis.CSharp;

class Program
{
    static void Main(string[] args)
    {
        foreach (var path in args)
        {
            Collect(path, File.ReadAllText(path), 0);
        }
    }

    static void Collect(string path, string text, int offset)
    {
        var root = CSharpSyntaxTree.ParseText(text).GetRoot();
        foreach (var trivia in root.DescendantTrivia(descendIntoTrivia: true))
        {
            switch (trivia.Kind())
            {
                case SyntaxKind.SingleLineCommentTrivia:
                    Print(path, offset + trivia.Span.Start, offset + trivia.Span.End, "line");
                    break;
                case SyntaxKind.MultiLineCommentTrivia:
                case SyntaxKind.MultiLineDocumentationCommentTrivia:
                    Print(path, offset + trivia.FullSpan.Start, offset + trivia.FullSpan.End, "block");
                    break;
                case SyntaxKind.DocumentationCommentExteriorTrivia:
                    var written = trivia.ToString();
                    if (!written.TrimStart().StartsWith("///"))
                    {
                        break;
                    }
                    var start = trivia.Span.Start + written.Length - written.TrimStart().Length;
                    var end = text.IndexOf('\n', start);
                    if (end < 0)
                    {
                        end = text.Length;
                    }
                    if (end > start && text[end - 1] == '\r')
                    {
                        end--;
                    }
                    Print(path, offset + start, offset + end, "line");
                    break;
                case SyntaxKind.DisabledTextTrivia:
                    Collect(path, trivia.ToFullString(), offset + trivia.FullSpan.Start);
                    break;
            }
        }
    }

    static void Print(string path, int start, int end, string kind) =>
        Console.WriteLine($"{path}\t{start}\t{end}\t{kind}");
}
