/**
 * Bowrain Editor add-on — Apps Script reference implementation.
 *
 * This is the *no-server* alternative to the HTTP card backend (../deployment.json,
 * served by bowrain-server's /api/v1/addin/google/* endpoints). It runs entirely
 * in Apps Script: it reads the active document with the Editor services
 * (DocumentApp / SpreadsheetApp / SlidesApp), calls the Bowrain add-in REST API
 * for brand/terminology/translation, and renders the result with CardService.
 *
 * Configure BOWRAIN_API and BOWRAIN_TOKEN (Project Settings → Script properties),
 * then deploy as an Editor add-on. Use this when you'd rather not host the HTTP
 * card endpoints; use the HTTP add-on for many users / central control.
 */

/** Base URL of a Bowrain server exposing /api/v1/addin (Script property). */
function apiBase_() {
  return (PropertiesService.getScriptProperties().getProperty('BOWRAIN_API') || '').replace(/\/$/, '');
}

/** Bearer token (bwt_…) for the Bowrain add-in REST API (Script property). */
function apiToken_() {
  return PropertiesService.getScriptProperties().getProperty('BOWRAIN_TOKEN') || '';
}

/** Homepage trigger — render the sidebar. */
function onHomepage(e) {
  return buildCard_(null, null, 'fr');
}

/** Read the active document's text across Docs / Sheets / Slides. */
function activeText_() {
  if (typeof DocumentApp !== 'undefined' && DocumentApp.getActiveDocument()) {
    return DocumentApp.getActiveDocument().getBody().getText();
  }
  if (typeof SpreadsheetApp !== 'undefined' && SpreadsheetApp.getActiveSpreadsheet()) {
    var values = SpreadsheetApp.getActiveSheet().getDataRange().getDisplayValues();
    return values.map(function (row) { return row.join('\t'); }).join('\n');
  }
  if (typeof SlidesApp !== 'undefined' && SlidesApp.getActivePresentation()) {
    var out = [];
    SlidesApp.getActivePresentation().getSlides().forEach(function (slide) {
      slide.getShapes().forEach(function (shape) {
        if (shape.getText) out.push(shape.getText().asString());
      });
    });
    return out.join('\n');
  }
  return '';
}

/** POST a JSON body to a Bowrain add-in endpoint. */
function callApi_(path, body) {
  var res = UrlFetchApp.fetch(apiBase_() + '/api/v1/addin' + path, {
    method: 'post',
    contentType: 'application/json',
    headers: apiToken_() ? { Authorization: 'Bearer ' + apiToken_() } : {},
    payload: JSON.stringify(body),
    muteHttpExceptions: true,
  });
  if (res.getResponseCode() >= 400) {
    throw new Error('Bowrain API ' + res.getResponseCode() + ': ' + res.getContentText());
  }
  return JSON.parse(res.getContentText());
}

/** Scan action — check brand voice + terminology, re-render the card. */
function onScan(e) {
  var text = activeText_();
  var check = callApi_('/check', { text: text });
  var terms = callApi_('/terms', { text: text });
  return buildCard_(check, terms, (e && e.formInput && e.formInput.targetLang) || 'fr');
}

/** Translate action — translate the whole document and replace its text. */
function onTranslate(e) {
  var target = (e && e.formInput && e.formInput.targetLang) || 'fr';
  var text = activeText_();
  var res = callApi_('/translate', { text: text, target_locale: target });
  if (typeof DocumentApp !== 'undefined' && DocumentApp.getActiveDocument()) {
    DocumentApp.getActiveDocument().getBody().setText(res.translation);
  }
  return CardService.newActionResponseBuilder()
    .setNotification(CardService.newNotification().setText('Translated to ' + target + '.'))
    .setNavigation(CardService.newNavigation().updateCard(buildCard_(null, null, target)))
    .build();
}

/** Build the sidebar card. */
function buildCard_(check, terms, target) {
  var builder = CardService.newCardBuilder()
    .setHeader(CardService.newCardHeader().setTitle('Bowrain').setSubtitle('Brand · terminology · translation'));

  // Findings.
  var findings = CardService.newCardSection().setHeader('Brand voice');
  if (check && check.findings && check.findings.length) {
    findings.setHeader('Brand voice · score ' + check.score);
    check.findings.forEach(function (f) {
      findings.addWidget(CardService.newDecoratedText()
        .setTopLabel(f.category + ' · ' + f.severity)
        .setText(f.message)
        .setWrapText(true));
    });
  } else if (check) {
    findings.addWidget(CardService.newTextParagraph().setText('No brand-voice issues found. ✔'));
  } else {
    findings.addWidget(CardService.newTextParagraph().setText('Run a scan to check this document.'));
  }
  builder.addSection(findings);

  // Terminology.
  if (terms && terms.matches && terms.matches.length) {
    var t = CardService.newCardSection().setHeader('Terminology');
    terms.matches.forEach(function (m) {
      t.addWidget(CardService.newDecoratedText().setText(m.term).setBottomLabel(m.status));
    });
    builder.addSection(t);
  }

  // Translate controls.
  var controls = CardService.newCardSection().setHeader('Translate');
  var picker = CardService.newSelectionInput()
    .setType(CardService.SelectionInputType.DROPDOWN)
    .setTitle('Target language')
    .setFieldName('targetLang');
  ['fr', 'de', 'es', 'ja', 'pt'].forEach(function (code) {
    picker.addItem(code, code, code === target);
  });
  controls.addWidget(picker);
  controls.addWidget(CardService.newButtonSet()
    .addButton(CardService.newTextButton().setText('Scan').setOnClickAction(CardService.newAction().setFunctionName('onScan')))
    .addButton(CardService.newTextButton().setText('Translate').setOnClickAction(CardService.newAction().setFunctionName('onTranslate'))));
  builder.addSection(controls);

  return builder.build();
}
