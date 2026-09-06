# Excluded inline HTML

A paragraph with <script>*emphasis*</script> inside a script span.

A paragraph with <style>[a link](https://example.com)</style> inside a style span.

Every inline construct at once: <script>`code` and <https://example.com> and ![i](s.png) and ~~struck~~</script> after it.

Nested excluded spans: <script>outer <style>*inner*</style> outer</script> after them.

Inline math is excluded too: <math><mi>a</mi><mo>+</mo><mi>b</mi></math> in a sentence.
