//! Prints the comments rustc's lexer finds in each file named on the command
//! line, one per line: the path, the start and end byte offsets, and `line` or
//! `block`. rust-goldens.py turns them into goldens.

use ra_ap_rustc_lexer::{strip_shebang, tokenize, FrontmatterAllowed, TokenKind};

fn main() {
    for path in std::env::args().skip(1) {
        let src = std::fs::read_to_string(&path).unwrap_or_else(|e| panic!("read {path}: {e}"));
        let mut offset = strip_shebang(&src).unwrap_or(0);
        for token in tokenize(&src[offset..], FrontmatterAllowed::No) {
            let len = token.len as usize;
            let kind = match token.kind {
                TokenKind::LineComment { .. } => Some("line"),
                TokenKind::BlockComment { terminated, .. } => {
                    assert!(terminated, "{path}: unterminated block comment at {offset}");
                    Some("block")
                }
                _ => None,
            };
            if let Some(kind) = kind {
                println!("{path}\t{offset}\t{}\t{kind}", offset + len);
            }
            offset += len;
        }
    }
}
