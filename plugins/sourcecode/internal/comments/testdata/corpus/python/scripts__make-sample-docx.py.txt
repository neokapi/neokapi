#!/usr/bin/env python3
"""Generate a minimal, valid .docx sample with an embedded image.

Produces web/static/samples/embedded-image.docx — a Word document with a
heading, a paragraph, and an inline image (the text-bearing vision sample). It
exercises the image-in-document story: the OOXML reader extracts the embedded
picture as a Media part (ExtractMedia), which the vision/alt-text/localization
paths can then work on.

Reproducible: re-run to regenerate. Embeds web/static/samples/vision-doc.png.
"""
import os
import struct
import zipfile

ROOT = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
IMG = os.path.join(ROOT, "web/static/samples/vision-doc.png")
OUT = os.path.join(ROOT, "web/static/samples/embedded-image.docx")


def png_size(path):
    """Return (width, height) of a PNG from its IHDR."""
    with open(path, "rb") as f:
        f.read(16)  # 8-byte sig + 4-byte len + "IHDR"
        w, h = struct.unpack(">II", f.read(8))
    return w, h


CONTENT_TYPES = """<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types">
  <Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/>
  <Default Extension="xml" ContentType="application/xml"/>
  <Default Extension="png" ContentType="image/png"/>
  <Override PartName="/word/document.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.document.main+xml"/>
</Types>"""

RELS = """<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">
  <Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="word/document.xml"/>
</Relationships>"""

DOC_RELS = """<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">
  <Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/image" Target="media/image1.png"/>
</Relationships>"""


def document_xml(cx, cy):
    return f"""<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"
  xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships"
  xmlns:wp="http://schemas.openxmlformats.org/drawingml/2006/wordprocessingDrawing"
  xmlns:a="http://schemas.openxmlformats.org/drawingml/2006/main"
  xmlns:pic="http://schemas.openxmlformats.org/drawingml/2006/picture">
  <w:body>
    <w:p><w:pPr><w:pStyle w:val="Heading1"/></w:pPr><w:r><w:t>Quarterly Report</w:t></w:r></w:p>
    <w:p><w:r><w:t>The figure below is an embedded image with text baked into the pixels.</w:t></w:r></w:p>
    <w:p><w:r><w:drawing>
      <wp:inline distT="0" distB="0" distL="0" distR="0">
        <wp:extent cx="{cx}" cy="{cy}"/>
        <wp:docPr id="1" name="Picture 1" descr="A quarterly results figure"/>
        <a:graphic><a:graphicData uri="http://schemas.openxmlformats.org/drawingml/2006/picture">
          <pic:pic>
            <pic:nvPicPr><pic:cNvPr id="1" name="image1.png" descr="A quarterly results figure"/><pic:cNvPicPr/></pic:nvPicPr>
            <pic:blipFill><a:blip r:embed="rId1"/><a:stretch><a:fillRect/></a:stretch></pic:blipFill>
            <pic:spPr><a:xfrm><a:off x="0" y="0"/><a:ext cx="{cx}" cy="{cy}"/></a:xfrm>
              <a:prstGeom prst="rect"><a:avLst/></a:prstGeom></pic:spPr>
          </pic:pic>
        </a:graphicData></a:graphic>
      </wp:inline>
    </w:drawing></w:r></w:p>
  </w:body>
</w:document>"""


def main():
    w, h = png_size(IMG)
    # 1 px ≈ 9525 EMU (96 DPI). Cap display width to ~5 in (4572000 EMU).
    cx = w * 9525
    cy = h * 9525
    max_cx = 4572000
    if cx > max_cx:
        cy = int(cy * max_cx / cx)
        cx = max_cx
    with zipfile.ZipFile(OUT, "w", zipfile.ZIP_DEFLATED) as z:
        z.writestr("[Content_Types].xml", CONTENT_TYPES)
        z.writestr("_rels/.rels", RELS)
        z.writestr("word/document.xml", document_xml(cx, cy))
        z.writestr("word/_rels/document.xml.rels", DOC_RELS)
        with open(IMG, "rb") as f:
            z.writestr("word/media/image1.png", f.read())
    print(f"wrote {OUT} ({os.path.getsize(OUT)} bytes, image {w}x{h})")


if __name__ == "__main__":
    main()
