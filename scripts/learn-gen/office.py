#!/usr/bin/env -S uv run --script
# /// script
# requires-python = ">=3.11"
# dependencies = [
#   "python-docx>=1.1,<2",
#   "openpyxl>=3.1,<4",
#   "python-pptx>=1.0,<2",
#   "pillow>=10,<13",
# ]
# ///
"""Write the Office documents the File conversion lab converts.

Three KapiMart documents, one per Office application, each built to carry the
structure the content model keeps across a format crossing: headings at two
levels, numbered and bulleted lists, a table with a header row, inline bold,
italics, strikethrough, superscript and hyperlinks, an embedded figure with alt
text, several worksheets with number, currency, percentage and date formats, and
a slide deck with a table and speaker notes.

The files land in web/static/samples/, where the docs site serves them at
/samples/<name>, and the lab (packages/kapi-learn/src/curriculum/labs/explore.ts)
names them chapter by chapter. The output is deterministic: document properties
carry a fixed date and every zip entry a fixed timestamp, so a regeneration that
changes nothing writes nothing new, and --check can hold the committed files to
the script.

    make learn-samples            # uv run scripts/learn-gen/office.py
    make learn-samples-check      # fail when the committed files are stale
"""

from __future__ import annotations

import argparse
import datetime as dt
import io
import pathlib
import re
import sys
import zipfile

from docx import Document
from docx.enum.style import WD_STYLE_TYPE
from docx.enum.text import WD_ALIGN_PARAGRAPH
from docx.oxml import OxmlElement
from docx.oxml.ns import qn
from docx.shared import Inches, Pt, RGBColor
from openpyxl import Workbook
from openpyxl.styles import Alignment, Font, PatternFill
from openpyxl.utils import get_column_letter
from openpyxl.worksheet.table import Table, TableStyleInfo
from PIL import Image, ImageDraw
from pptx import Presentation
from pptx.util import Inches as PInches
from pptx.util import Pt as PPt

HERE = pathlib.Path(__file__).resolve().parent
REPO = HERE.parent.parent
DEFAULT_OUT = REPO / "web" / "static" / "samples"

# One instant for every document property, so a regeneration diffs on content.
STAMP = dt.datetime(2026, 9, 1, 9, 0, 0)
AUTHOR = "KapiMart Partner Team"

# The fee schedule, shared by the handbook table and the deck's table slide.
FEES = [
    ("Plan", "Monthly fee", "Transaction fee", "Payout schedule"),
    ("Starter", "€0", "8.0 %", "Monthly"),
    ("Growth", "€49", "6.5 %", "Fortnightly"),
    ("Scale", "€199", "5.0 %", "Weekly"),
    ("Enterprise", "Custom", "Custom", "Daily"),
]


# ── zip determinism ──────────────────────────────────────────────────────────


MODIFIED_RE = re.compile(rb"(<dcterms:modified[^>]*>)[^<]*(</dcterms:modified>)")


def normalise_zip(data: bytes) -> bytes:
    """Rewrite an OOXML package with a fixed timestamp on every entry.

    The three libraries stamp each zip entry with the wall clock, and openpyxl
    writes the save time into the modified property whatever the workbook says,
    so two runs of the same script differ byte for byte. Entry order and content
    are kept.
    """
    src = zipfile.ZipFile(io.BytesIO(data))
    out = io.BytesIO()
    stamp = STAMP.strftime("%Y-%m-%dT%H:%M:%SZ").encode()
    with zipfile.ZipFile(out, "w") as dst:
        for item in src.infolist():
            payload = src.read(item.filename)
            if item.filename == "docProps/core.xml":
                payload = MODIFIED_RE.sub(rb"\g<1>" + stamp + rb"\g<2>", payload)
            info = zipfile.ZipInfo(item.filename, date_time=(1980, 1, 1, 0, 0, 0))
            info.compress_type = zipfile.ZIP_DEFLATED
            info.external_attr = item.external_attr
            dst.writestr(info, payload)
    return out.getvalue()


# ── the figure the handbook embeds ───────────────────────────────────────────


def build_chart_png() -> bytes:
    """A bar chart of quarterly partner payouts, drawn without a font file."""
    values = [0.9, 1.4, 1.9, 2.3, 2.8, 3.4, 3.9, 4.6, 5.2, 6.1, 6.9, 7.8]
    labels = ["Q1 24", "Q2", "Q3", "Q4", "Q1 25", "Q2", "Q3", "Q4", "Q1 26", "Q2", "Q3", "Q4"]
    w, h = 960, 480
    left, right, top, bottom = 80, 30, 40, 70
    img = Image.new("RGB", (w, h), "white")
    d = ImageDraw.Draw(img)
    d.text((left, 12), "Partner payouts per quarter, EUR million", fill=(40, 40, 40))
    axis = (120, 120, 120)
    d.line([(left, top), (left, h - bottom)], fill=axis, width=2)
    d.line([(left, h - bottom), (w - right, h - bottom)], fill=axis, width=2)
    top_value = 8.0
    for tick in range(0, 9, 2):
        y = h - bottom - (tick / top_value) * (h - top - bottom)
        d.line([(left - 6, y), (left, y)], fill=axis, width=2)
        d.text((left - 40, y - 6), f"{tick}", fill=(90, 90, 90))
    slot = (w - left - right) / len(values)
    for i, (v, label) in enumerate(zip(values, labels)):
        x0 = left + i * slot + slot * 0.2
        x1 = left + (i + 1) * slot - slot * 0.2
        y0 = h - bottom - (v / top_value) * (h - top - bottom)
        d.rectangle([(x0, y0), (x1, h - bottom - 1)], fill=(47, 107, 79))
        d.text((x0 + 2, h - bottom + 10), label, fill=(90, 90, 90))
    buf = io.BytesIO()
    img.save(buf, format="PNG", optimize=True)
    return buf.getvalue()


# ── Word: the partner handbook ───────────────────────────────────────────────


def add_hyperlink(paragraph, text: str, url: str) -> None:
    part = paragraph.part
    r_id = part.relate_to(
        url,
        "http://schemas.openxmlformats.org/officeDocument/2006/relationships/hyperlink",
        is_external=True,
    )
    link = OxmlElement("w:hyperlink")
    link.set(qn("r:id"), r_id)
    run = OxmlElement("w:r")
    props = OxmlElement("w:rPr")
    style = OxmlElement("w:rStyle")
    style.set(qn("w:val"), "Hyperlink")
    props.append(style)
    run.append(props)
    t = OxmlElement("w:t")
    t.text = text
    t.set(qn("xml:space"), "preserve")
    run.append(t)
    link.append(run)
    paragraph._p.append(link)


def mark_header_row(row) -> None:
    """Flag a table row as a header row (w:tblHeader), so readers keep it as one."""
    tr_pr = row._tr.get_or_add_trPr()
    header = OxmlElement("w:tblHeader")
    header.set(qn("w:val"), "true")
    tr_pr.append(header)


def build_docx(chart_png: bytes) -> bytes:
    doc = Document()
    cp = doc.core_properties
    cp.author = AUTHOR
    cp.last_modified_by = AUTHOR
    cp.title = "KapiMart Partner Handbook"
    cp.subject = "Onboarding, fees and support for marketplace partners"
    cp.comments = "The handbook every new KapiMart partner receives."
    cp.created = STAMP
    cp.modified = STAMP
    cp.revision = 1

    # Word's own character style for a link, which the template leaves out.
    link_style = doc.styles.add_style("Hyperlink", WD_STYLE_TYPE.CHARACTER)
    link_style.font.color.rgb = RGBColor(0x05, 0x63, 0xC1)
    link_style.font.underline = True

    doc.add_heading("KapiMart Partner Handbook", level=0)
    doc.add_paragraph(
        "Everything a new marketplace seller needs in the first ninety days", style="Subtitle"
    )
    edition = doc.add_paragraph()
    edition.add_run("Edition 2026.3, published by the KapiMart Partner Team").italic = True

    doc.add_heading("1. Welcome to the marketplace", level=1)
    p = doc.add_paragraph("KapiMart connects independent food producers with more than ")
    p.add_run("two million").bold = True
    p.add_run(
        " active buyers across Europe. This handbook walks you through onboarding, the fee "
        "schedule and the support channels, and points to the "
    )
    add_hyperlink(p, "Partner Portal", "https://partners.kapimart.example.com")
    p.add_run(" for the day-to-day work.")
    doc.add_paragraph("Read it once from start to end; after that, each chapter stands on its own.")

    doc.add_heading("2. Getting started", level=1)
    doc.add_heading("2.1 Five steps to your first order", level=2)
    for step in [
        "Create your Seller Account in the Partner Portal and verify your business identity.",
        "Complete the business profile: legal name, VAT number and the address that appears on invoices.",
        "Upload your product catalogue with the bulk import template, or add products one at a time.",
        "Set your shipping zones, delivery promises and return policy.",
        "Connect a payout account and submit your first listing for review.",
    ]:
        doc.add_paragraph(step, style="List Number")

    doc.add_heading("2.2 Account requirements", level=2)
    for item in [
        "A registered business in a supported country",
        "Food-safety certification for every product category you list",
        "A customer-service mailbox answered within one business day",
        "Product photographs of at least 1200 × 1200 pixels",
    ]:
        doc.add_paragraph(item, style="List Bullet")

    doc.add_heading("3. Fees and payouts", level=1)
    p = doc.add_paragraph("Fees depend on the plan. ")
    p.add_run("The Starter plan carried a €19 monthly fee until June 2026").font.strike = True
    p.add_run("; ")
    p.add_run("Starter is now free").bold = True
    p.add_run(", and the transaction fee is unchanged.")
    p.add_run("1").font.superscript = True

    table = doc.add_table(rows=len(FEES), cols=len(FEES[0]))
    table.style = "Table Grid"
    for r, cells in enumerate(FEES):
        row = table.rows[r]
        for c, text in enumerate(cells):
            cell = row.cells[c]
            cell.text = ""
            run = cell.paragraphs[0].add_run(text)
            if r == 0:
                run.bold = True
    mark_header_row(table.rows[0])

    doc.add_paragraph(
        "Payouts are made in euros to the account on file. Partners outside the euro area "
        "receive the amount converted at the European Central Bank reference rate on the "
        "payout date."
    )

    shape = doc.add_picture(io.BytesIO(chart_png), width=Inches(5.8))
    doc_pr = shape._inline.docPr
    doc_pr.set("name", "Figure 1")
    doc_pr.set("descr", "Bar chart of quarterly partner payouts from 2024 to 2026")
    caption = doc.add_paragraph()
    caption.add_run("Figure 1. Quarterly partner payouts since the marketplace opened.").italic = True
    caption.alignment = WD_ALIGN_PARAGRAPH.CENTER

    note = doc.add_paragraph()
    note.add_run("1").font.superscript = True
    small = note.add_run(" Fees quoted exclude VAT; the Partner Agreement carries the full schedule.")
    small.font.size = Pt(9)

    doc.add_heading("4. Support", level=1)
    p = doc.add_paragraph("Partner Support is open Monday to Friday, 08:00 to 18:00 CET. Write to ")
    add_hyperlink(p, "partners@kapimart.example.com", "mailto:partners@kapimart.example.com")
    p.add_run(
        " or open a ticket from the Portal. Urgent order issues go to the on-call line printed "
        "on your dashboard."
    )
    doc.add_paragraph(
        "Our best partners read this handbook twice: once before the first listing, and once "
        "after the first hundred orders.",
        style="Intense Quote",
    )

    buf = io.BytesIO()
    doc.save(buf)
    return normalise_zip(buf.getvalue())


# ── Excel: the regional sales workbook ───────────────────────────────────────

EUR0 = '"€"#,##0'
EUR2 = '"€"#,##0.00'
INT = "#,##0"
PCT = "0.0%"
DATE = "yyyy-mm-dd"

HEADER_FILL = PatternFill("solid", fgColor="2F6B4F")
HEADER_FONT = Font(bold=True, color="FFFFFF")


def write_table(ws, origin_row, header, rows, formats, name) -> None:
    for c, title in enumerate(header, start=1):
        cell = ws.cell(row=origin_row, column=c, value=title)
        cell.font = HEADER_FONT
        cell.fill = HEADER_FILL
        cell.alignment = Alignment(horizontal="left")
    for r, values in enumerate(rows, start=origin_row + 1):
        for c, value in enumerate(values, start=1):
            cell = ws.cell(row=r, column=c, value=value)
            fmt = formats[c - 1]
            if fmt:
                cell.number_format = fmt
    last_col = get_column_letter(len(header))
    ref = f"A{origin_row}:{last_col}{origin_row + len(rows)}"
    table = Table(displayName=name, ref=ref)
    table.tableStyleInfo = TableStyleInfo(name="TableStyleMedium2", showRowStripes=True)
    ws.add_table(table)


def autosize(ws) -> None:
    widths: dict[int, int] = {}
    for row in ws.iter_rows():
        for cell in row:
            if cell.value is None:
                continue
            text = cell.value.isoformat() if isinstance(cell.value, dt.date) else str(cell.value)
            widths[cell.column] = max(widths.get(cell.column, 0), len(text))
    for col, width in widths.items():
        ws.column_dimensions[get_column_letter(col)].width = min(max(width + 2, 10), 40)


def build_xlsx() -> bytes:
    wb = Workbook()
    props = wb.properties
    props.creator = "KapiMart Finance"
    props.lastModifiedBy = "KapiMart Finance"
    props.title = "KapiMart marketplace, regional sales Q3 2026"
    props.created = STAMP
    props.modified = STAMP

    ws = wb.active
    ws.title = "Summary"
    ws["A1"] = "KapiMart marketplace, Q3 2026 (1 July to 30 September), revenue in euros"
    ws["A1"].font = Font(bold=True, size=14)
    write_table(
        ws,
        3,
        ["Metric", "Q3 2026", "Q3 2025", "Change"],
        [
            ["Gross merchandise value", 12480250, 9815600, 0.271],
            ["Orders", 418930, 352110, 0.190],
            ["Active partners", 2146, 1603, 0.339],
            ["Average order value", 29.79, 27.88, 0.069],
            ["Repeat-buyer share", 0.46, 0.41, 0.05],
        ],
        [None, None, None, PCT],
        "Summary",
    )
    for row, fmt in ((4, EUR0), (5, INT), (6, INT), (7, EUR2), (8, PCT)):
        for col in ("B", "C"):
            ws[f"{col}{row}"].number_format = fmt
    autosize(ws)
    ws.freeze_panes = "A4"

    ws = wb.create_sheet("By region")
    write_table(
        ws,
        1,
        ["Region", "Orders", "Revenue", "Average order", "Growth", "Top category"],
        [
            ["Nordics", 96420, 2931150, 30.40, 0.312, "Pantry essentials"],
            ["DACH", 118300, 3562480, 30.11, 0.248, "Spices"],
            ["Benelux", 61250, 1784920, 29.14, 0.189, "Beverages"],
            ["France", 58760, 1702330, 28.97, 0.225, "Cooking oils"],
            ["Iberia", 47900, 1296610, 27.07, 0.354, "Dried fruits"],
            ["British Isles", 36300, 1202760, 33.13, 0.151, "Confectionery"],
            ["Total", 418930, 12480250, 29.79, 0.271, ""],
        ],
        [None, INT, EUR0, EUR2, PCT, None],
        "Regions",
    )
    for cell in ws[8]:
        cell.font = Font(bold=True)
    autosize(ws)
    ws.freeze_panes = "A2"

    ws = wb.create_sheet("Top products")
    write_table(
        ws,
        1,
        ["Product", "SKU", "Category", "Units", "Revenue", "Launched"],
        [
            ["Organic Honey Jar", "KM-HON-001", "Pantry essentials", 38420, 498850, dt.date(2024, 3, 12)],
            ["Artisan Olive Oil", "KM-OIL-002", "Cooking oils", 29110, 538540, dt.date(2023, 11, 2)],
            ["Matcha Powder", "KM-MAT-014", "Beverages", 15980, 479240, dt.date(2025, 2, 18)],
            ["Dark Chocolate Bar", "KM-CHO-004", "Confectionery", 92300, 460580, dt.date(2022, 9, 30)],
            ["Maple Syrup Bottle", "KM-MAP-010", "Pantry essentials", 24760, 371150, dt.date(2024, 6, 5)],
            ["Turmeric Powder", "KM-TUR-011", "Spices", 33210, 331770, dt.date(2023, 5, 22)],
            ["Acai Berry Pack", "KM-ACA-012", "Superfoods", 12040, 276800, dt.date(2025, 7, 1)],
            ["Granola Mix", "KM-GRA-013", "Breakfast", 27500, 261000, dt.date(2024, 10, 14)],
        ],
        [None, None, None, INT, EUR0, DATE],
        "TopProducts",
    )
    autosize(ws)
    ws.freeze_panes = "A2"

    buf = io.BytesIO()
    wb.save(buf)
    return normalise_zip(buf.getvalue())


# ── PowerPoint: the partner kickoff deck ─────────────────────────────────────


def bullets(slide, items) -> None:
    body = slide.placeholders[1].text_frame
    body.clear()
    first = True
    for item in items:
        p = body.paragraphs[0] if first else body.add_paragraph()
        first = False
        if isinstance(item, tuple):
            lead, rest = item
            r = p.add_run()
            r.text = lead
            r.font.bold = True
            r2 = p.add_run()
            r2.text = rest
        else:
            p.text = item
        p.font.size = PPt(24)


def build_pptx() -> bytes:
    prs = Presentation()
    prs.slide_width = PInches(13.333)
    prs.slide_height = PInches(7.5)
    cp = prs.core_properties
    cp.author = AUTHOR
    cp.last_modified_by = AUTHOR
    cp.title = "KapiMart partner kickoff"
    cp.created = STAMP
    cp.modified = STAMP
    cp.revision = 1

    title_layout = prs.slide_layouts[0]
    content_layout = prs.slide_layouts[1]
    title_only = prs.slide_layouts[5]

    s = prs.slides.add_slide(title_layout)
    s.shapes.title.text = "Partner kickoff"
    s.placeholders[1].text = "KapiMart marketplace, autumn 2026 cohort"

    s = prs.slides.add_slide(content_layout)
    s.shapes.title.text = "Agenda"
    bullets(
        s,
        [
            "Who KapiMart is",
            "What a partner gets",
            "Fees at a glance",
            "Your first ninety days",
            "Questions",
        ],
    )
    s.notes_slide.notes_text_frame.text = (
        "Forty minutes in all. Keep the fee slide short; the handbook has the detail."
    )

    s = prs.slides.add_slide(content_layout)
    s.shapes.title.text = "What a partner gets"
    bullets(
        s,
        [
            ("Reach", ": two million active buyers in six regions"),
            ("Tools", ": catalogue import, order management and analytics"),
            ("Support", ": a named onboarding specialist for ninety days"),
            ("Payouts", ": weekly, in euros, with a statement for every transfer"),
        ],
    )

    s = prs.slides.add_slide(title_only)
    s.shapes.title.text = "Fees at a glance"
    rows, cols = len(FEES), len(FEES[0])
    shape = s.shapes.add_table(rows, cols, PInches(1.0), PInches(1.8), PInches(11.3), PInches(3.2))
    table = shape.table
    for r, cells in enumerate(FEES):
        for c, text in enumerate(cells):
            cell = table.cell(r, c)
            cell.text = text
            for p in cell.text_frame.paragraphs:
                p.font.size = PPt(20)
                if r == 0:
                    p.font.bold = True

    s = prs.slides.add_slide(content_layout)
    s.shapes.title.text = "Your first ninety days"
    bullets(
        s,
        [
            ("Days 1 to 10", ": account, business profile and a first listing"),
            ("Days 11 to 30", ": the full catalogue and your shipping zones"),
            ("Days 31 to 60", ": a first promotion, then read your analytics"),
            ("Days 61 to 90", ": the quarterly review with your specialist"),
        ],
    )

    s = prs.slides.add_slide(title_layout)
    s.shapes.title.text = "Thank you"
    s.placeholders[1].text = "partners@kapimart.example.com\npartners.kapimart.example.com"

    buf = io.BytesIO()
    prs.save(buf)
    return normalise_zip(buf.getvalue())


# ── entry point ──────────────────────────────────────────────────────────────


def rel(path: pathlib.Path) -> str:
    return str(path.relative_to(REPO)) if path.is_relative_to(REPO) else str(path)


def main() -> int:
    ap = argparse.ArgumentParser(description=__doc__.split("\n\n")[0])
    ap.add_argument("--out", type=pathlib.Path, default=DEFAULT_OUT, help="output directory")
    ap.add_argument(
        "--check", action="store_true", help="exit 1 when a committed file differs from the output"
    )
    args = ap.parse_args()

    chart = build_chart_png()
    files = {
        "kapimart-partner-handbook.docx": build_docx(chart),
        "kapimart-regional-sales.xlsx": build_xlsx(),
        "kapimart-partner-kickoff.pptx": build_pptx(),
    }

    stale = []
    args.out.mkdir(parents=True, exist_ok=True)
    for name, data in files.items():
        path = args.out / name
        current = path.read_bytes() if path.exists() else None
        if current == data:
            print(f"unchanged  {rel(path)}")
            continue
        if args.check:
            stale.append(name)
            continue
        path.write_bytes(data)
        print(f"wrote      {rel(path)}  ({len(data)} bytes)")
    if stale:
        print(
            "stale: " + ", ".join(stale) + "\nrun `make learn-samples` and commit the result",
            file=sys.stderr,
        )
        return 1
    return 0


if __name__ == "__main__":
    sys.exit(main())
