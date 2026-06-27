# Artifact Preview Stale State Design

## Goal

Prevent stale artifact previews from remaining visible after a later preview
action fails or routes to a non-inline renderer.

M4.41 clears the drawer inline preview at the start of every preview action.
Download actions do not clear the current preview.

## Behavior

When the user clicks `预览`:

1. clear the current inline preview state;
2. execute the preview route for the target artifact;
3. render a new inline preview only if the target route completes as text,
   table, or raster image;
4. show a bounded drawer error if the target route fails;
5. leave the preview area empty on failure.

This avoids showing a previous image or text preview while the drawer reports
that a different artifact preview failed.

## Non-Goals

M4.41 does not add retry UI, browser E2E, PDF embedding, skeleton previews, or
signed URL refresh.
