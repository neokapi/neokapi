# Pageglass: factual specification

Pageglass is a fictional documentation-preview tool for this experiment. These
facts define the product; they are not instructions about writing style. There
is no published package or executable to install or test. The intended readers
are software developers and small documentation teams who can use a terminal
but have not used Pageglass.

## Purpose and supported content

Pageglass lets a writer inspect a documentation change locally, then share a
fixed preview with reviewers. It reads ordinary Markdown files and relative
PNG, JPEG and SVG images from one input directory. It preserves relative links
between pages. It does not execute JavaScript, support MDX components, run a
framework build, or publish a production website. It has no comments, approval
workflow, repository integration or automatic deployment.

## Local preview

Assume the writer already has a configured Pageglass installation and a `docs/`
directory. Installation and account setup are outside this documentation task;
do not invent installation, login or configuration commands.

Run `pageglass preview docs --port 4400` from the parent of `docs/`. This builds
a local preview, prints a preview ID and the address `http://127.0.0.1:4400`,
and watches the input directory while the process runs. The server listens only
on the local machine. It does not upload content. Saving a supported source
file rebuilds the local preview. Refresh the browser to see the new version;
there is no automatic browser refresh. Ctrl+C stops the server. The default
port is 4400; `--port 4401` selects a different port when needed.

A preview ID identifies the local preview session. An example ID is
`pv_example`. Its current successfully built snapshot remains available for
sharing after the local server stops. A failed rebuild leaves the previous
successful snapshot available; it does not replace it with the broken version.

## Shared previews

Sharing requires network access and an already configured workspace account.
Run `pageglass share pv_example --expires 24h` to upload the current successful
snapshot and create a new preview link. `pv_example` is an example: substitute
the ID printed by the local preview command. The output includes a link ID
(for example `ln_example`) and an HTTPS address on the configured workspace.
Use `https://preview.example/p/example` only as a clearly marked illustrative
address. The `.example` address is not a live service.

The default expiry is 24 hours. `--expires` accepts `1h`, `24h`, or `7d` only.
A shared preview is a fixed snapshot. Later local edits do not change an
existing link. Share again to obtain a new link for a newer snapshot. An upload
failure creates no new link and leaves the local preview available.

Anyone holding a shared URL can view it without signing in. It is not a
private, account-restricted review space. Share only material suitable for
everyone who may receive the link. Shared content is uploaded to the configured
workspace; local-only preview content stays on the writer's machine.

Run `pageglass revoke ln_example` to revoke that link ID. Revocation stops new
requests to the link. It cannot remove copies a reviewer already downloaded.
Expiry also stops new requests. The documentation may explain these outcomes
but must not promise complete deletion or recovery of downloaded copies.

## Errors and recovery

- `PORT_IN_USE`: another process uses the selected port. Stop that process or
  run the preview with a different `--port` value.
- `UNSUPPORTED_SOURCE`: the directory contains MDX or another unsupported source
  type. Convert the relevant content to ordinary Markdown or remove that source
  from the input directory, then rerun the preview. Do not suggest installing a
  plugin or adding MDX support.
- `UPLOAD_FAILED`: sharing did not create a link. Check the network and the
  configured workspace availability, then repeat the share command. The local
  preview remains available.

No additional commands, flags, guarantees or capabilities are established by
this specification. Examples may introduce ordinary sample filenames, page
text and relative image paths. Keep product commands, supported option values
and behavior consistent with the facts above.
