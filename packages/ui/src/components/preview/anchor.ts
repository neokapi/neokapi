// anchor — the run-anchor arithmetic, which lives in @neokapi/kapi-format so
// every surface (web, desktop, mobile) maps an anchor the same way. It mirrors
// core/model/anchor.go and model.RunsText.

export {
  charSpanForAnchor,
  rangeAnchorForBytes,
  rangeAnchorForChars,
  runPosOf,
  runsPlainText,
  textForBytes,
  type CharSpan,
} from "@neokapi/kapi-format";
