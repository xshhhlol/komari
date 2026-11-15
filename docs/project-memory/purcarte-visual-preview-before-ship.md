---
name: purcarte-visual-preview-before-ship
description: "For PurCarte look-and-feel changes, show rendered preview images (mockups) and get the user's pick BEFORE building/deploying — overrides the no-ask auto pipeline for taste decisions"
metadata:
  node_type: memory
  type: feedback
  originSessionId: 07752cb6-bc7c-4e2a-ae57-84fd17faa430
  modified: 2026-10-14T03:50:39.247Z
---

For PurCarte changes that are about **how something looks** (colors, button styles, layout polish), first render previews on the real page and show them to the user. Ship only after they pick one.

- How: run the mock backend and drive headless Chrome via CDP. Apply candidate styles inline to the real elements, crop the region, and compose a labelled sheet with PIL (normal + hover columns, one row per option). Use the font `/System/Library/Fonts/Hiragino Sans GB.ttc` for Chinese labels; PingFang is not present. Then `open` the PNG so it pops up in Preview, and give its path.
- After the user picks an option, run the full [[purcarte-theme-build-deploy]] pipeline without asking again.

**Why:** On 2026-10-14 I shipped three button restyles in a row; each one needed a release and a server upgrade, and the user disliked each. They said "最好是给出修改后的效果图我看下，不然我每次改动上线都很麻烦".

**How to apply:** This is an exception to [[forked-repos-full-pipeline-no-ask]] for visual-taste decisions only. Functional changes still go straight through the pipeline. The style history is in [[purcarte-button-style-restrained]].
