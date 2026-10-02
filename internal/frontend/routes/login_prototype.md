# Throwaway landing study — UNB-77

**Question:** can the login become a light, scroll-down introduction without burying sign-in or adding a wall of marketing copy?

## Start and stop

Use branch `prototype/UNB-77-login-landing` with the prerequisites from the
[README quickstart](../../../README.md#quickstart). From the repository root,
stop any existing `task dev`, then run:

```sh
task prototype:landing
```

Keep that terminal open. Browse port **7331**, not the underlying app port.
Stop the prototype with **Ctrl+C**. Do not run `task templ`, `task build`, or the
browser test tasks while the watch server is running.

## Open a variant

The query parameter is **`variant`**, with uppercase values **`A`**, **`B`**, or **`C`**:

| URL | Direction |
| --- | --- |
| <http://localhost:7331/login?variant=A> | **Quiet reveal:** familiar centered login; alternating scenes below the fold. |
| <http://localhost:7331/login?variant=B> | **Editorial:** README-style headline and tilted planner; type ribbon and broad workbench. |
| <http://localhost:7331/login?variant=C> | **Field guide:** playable plan up front; indexed chapters for planning, Jotpad, and habits. |

Use the floating bottom arrows or keyboard **← / →** to switch; navigation wraps
between A and C. Arrow keys inside inputs and the Jotpad keep their normal behavior.
The URL survives reload and can be bookmarked.

If you see the original login, check the `/login?variant=A` URL and restart using
`task prototype:landing`, not plain `task dev`. Missing or invalid variants render
the ordinary login; a query parameter alone does not enable prototypes.

## Manual review checklist

1. Compare the opening screen in each variant. Is sign-in easy to find, and does
   the downward arrow make the content below obvious? Click it and scroll through.
2. Try desktop and a narrow phone viewport (for example, 390 × 844). Check that
   headings, controls, and figures fit and the bottom switcher stays usable.
3. Drag **Deep Work**, **Email**, or **Coffee** in the interactive plan. Stretch
   a block using its bottom grip. A/B place the interactive plan below the opening;
   C puts it in the opening. Tilted illustrations are not interactive.
4. Type in the Jotpad and toggle habit circles. Reload: these edits and plan moves
   should reset. Switching variants also resets them.
5. Enter a valid-looking email and click **Send code**. It should remain a preview,
   send no email, and make no POST request (check browser DevTools → Network).
6. Focus the email field or Jotpad and press **← / →**: the variant should not change.
   Move focus outside them and verify that the switcher wraps through all variants.
7. Open DevTools → Console to see the current variant, block placements, note text,
   and habit states after interactions.
8. Open <http://localhost:7331/login> without the query parameter: the original,
   **real** login form should appear without a prototype switcher. Do not submit
   that form when checking the preview-only behavior above.

### Check dark mode

The prototypes inherit the app's existing stored theme. To preview dark mode,
run this in the browser console, then reload:

```js
localStorage.setItem('colormode', 'dark');
location.reload();
```

Use `'light'` to switch back. Unlike demo edits, this existing theme preference
persists in this browser.

## Scope and capture

Uses the existing theme, block tokens, logo, guide miniature and drag/stretch driver.
Login is a visual stub: it sends no email. Notes, habits, and demo placements are in memory only.
The console prints the relevant state on load and demo edits. The scratch database is
`tmp/PROTOTYPE-wipe-me-landing.db`; no real app data is needed.

Normal `/login` is unchanged. Variants require both `LANDING_PROTOTYPE=1` and
`TEMPL_DEV_MODE`; the task enables them. No production route or switcher is enabled.

**Verdict: A selected.** Its opening now matches the existing login exactly,
with only the scroll text and arrow added. Desktop and mobile presentation parity
and preview-only submission were verified. Continue implementation on
`feat/landingPage`; preserve this full study on `prototype/UNB-77-login-landing`.
