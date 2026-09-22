# 007 — Apple iPhone/iPad App Store launch

Status: research only; no implementation, issue, enrollment, or submission performed  
Date / primary-source access date: **2026-09-21**  
Repository inspected: `504b2f7`  
Question: What is the leanest defensible path from unbusy.day's hosted Go application to an approved iPhone/iPad app, and should its host be Capacitor or Wails v3?

## Recommendation

**Choose Capacitor for the first mobile spike, keep Go business logic and SQLite on Fly, and treat App Store readiness as a product/security project—not a packaging command.** Prefer a production app with bundled presentation assets, a small native integration surface, and the existing server-rendered components and mutation services behind a deliberately designed mobile transport. Do not rewrite the planner as a client-side business-logic SPA or copy the database onto devices.

There is an important cost boundary: this repository is **not a static web app**. Loading its existing HTTPS site is substantially easier than bundling it. Capacitor's `server.url` can demonstrate the remote-site approach, but its documentation explicitly says that setting is for live reload and **not intended for production**; `allowNavigation` has the same warning.[C2] Do not turn that demo configuration into the release architecture by accident. First time-box a comparison of remote-origin reuse against bundled-shell integration. If the latter becomes a second frontend or an elaborate HTTP proxy, consider a deliberately maintained, small Swift/WKWebView remote-content host instead, or retain the PWA. That is an engineering alternative, not a claim of automatic Apple acceptance.

**Wails v3 is no longer “desktop only.”** Its current official source documents working iOS and Android paths; the iOS guide explicitly labels support **experimental**, and the release feed's newest returned release is `v3.0.0-beta.24`, published September 20, 2026, marked prerelease.[W1–W4] Its Go-on-device advantage does not solve this project's main problem: the authoritative Go code should remain on Fly. Wails is a credible experimental alternative, but not my lowest-risk first launch choice.

For v1, propose the existing Plan/Jotpad/Habits experience, reliable session/resume handling, accessible phone/tablet layouts, deletion/privacy/support, and **one useful native feature: an optional daily planning reminder scheduled on the device**. Haptics can polish block placement, but neither haptics nor a reminder is an approval token. Apple guideline 4.2 requires utility and an experience beyond a repackaged website; **no framework or feature checklist guarantees approval**.[A1 §4.2]

## Evidence and limitations

- **Verified policy:** requirements read directly from Apple; cited with guideline sections or exact documentation URLs in the bibliography.
- **Verified framework/repository fact:** observed docs/source, not a claim that we built or tested it.
- **Recommendation:** proposed engineering/product work, not Apple policy.
- **Risk / unknown:** matters needing a device experiment, owner decision, dependency audit, or App Review judgment.

Primary pages were fetched with HTTP tools and read as text; Apple documentation's JSON representations were used where the HTML was only a JavaScript shell.[A7,A8,B3] Wails' public v3 home page returned **403**; its official GitHub documentation, tagged implementation, and release API were readable. The Wails documentation snapshot was `4b1f348438d374e66892c9aeba7809a497f1edef`; iOS support was also checked at the actual `v3.0.0-beta.24` tag, not inferred solely from unreleased main.[W1–W4] Capacitor pages served the **v8** documentation; a v9 navigation link is not evidence of a stable v9 release.[C1,C2]

Research was delegated to a background agent. No Xcode archive, simulator, physical-device test, authenticated production inspection, App Store Connect form, or App Review decision was obtained. Browser release notes establish individual engine features, **not end-to-end compatibility of this app**. Findings and submission requirements must be rechecked when implementation/submission actually begins. The existing untracked `006-proton-calendar-ics-feed.md` is outside this research and was preserved.

## 1. What we are actually shipping

These are local repository findings, not external product claims:

| Current implementation | Mobile consequence |
| --- | --- |
| `cmd/unbusy/main.go`, `router.go`, and `AGENTS.md`: Go services over one SQLite database, user-keyed in-process broker, authenticated HTML and SSE endpoints. | Keep the single-machine Fly authority. A native shell is another client, not another database writer implementation. |
| `internal/frontend/layouts/layout.templ`: server HTML, root-relative static paths, theme localStorage, Datastar **v1.0.2 from jsDelivr**, and service-worker registration. | Copying `static/` into a native bundle does not produce a working app. The initial document and authenticated fragments are generated on the server. Audit remote script dependencies too. |
| `internal/frontend/events.go`: initial block snapshot, Jotpad version/text signals, habit invalidation; 25-second heartbeats. `internal/frontend/routes/blocks.templ` owns stream initialization. | Preserve snapshot-on-reconnect and ordered habit re-reads. An HTTP streaming adapter must not buffer the whole response. |
| `internal/frontend/static/js/jot/sync.js`: debounced JSON CAS writes, retries, in-memory dirty state, visibility/pagehide beacons. | There is convergence logic, but no durable offline edit queue. A beacon invocation is not proof of a committed save. |
| `internal/frontend/static/sw.js`: passthrough only, no cache. | The PWA has no implemented offline application mode. Do not advertise native offline editing merely because assets are bundled. |
| `internal/web/session.go`, `internal/auth/auth.go`: HttpOnly `SameSite=Lax` cookie, Secure controlled by deployment configuration, absolute 30-day session; email OTP is short-lived (10 minutes), and successful first verification creates the account. | Same-origin remote browsing preserves the present model most directly. A different local web origin needs an explicit authentication/transport design. |
| `internal/frontend/components/login.templ`, `cmd/unbusy/auth.go`: Turnstile, email delivery and send guards; OTP numeric input and `autocomplete="one-time-code"`. | Test the real challenge and production mail path inside WKWebView. Existing input hints are not evidence that email-code autofill works on every device. |
| `internal/frontend/static/js/blocks/pointer.js`, `jot/cm.js`, `invoker-fallback.js`, `static/css/app.css`: pointer capture/cancellation, gesture/SSE arbitration, CodeMirror selection handling, dialog fallbacks, extensive CSS `@scope`. | Native packaging does not remove WebKit, keyboard, or gesture compatibility work. |
| `cmd/unbusy/router.go`: logout exists; no account-deletion, privacy-policy, or support route is mounted. | These are launch gaps in the inspected app. Separate externally hosted policy/support pages might exist, but were not verified. |

The domain is a **rolling today**, not dated plan history; habits have dated check-ins, and the Jotpad is a perpetual private document. Bounds currently run between 04:00 and 18:00 in half-hour slots. Store copy, screenshots and reminders must reflect those limits, not imply calendar synchronization, locked appointments or recurring dated plans.[R1]

## 2. Capacitor versus Wails v3: verified capabilities, not reputation

| Question | Capacitor | Wails v3 |
| --- | --- | --- |
| iPhone/iPad feasibility | Official iOS target, Swift/Objective-C bridge, WKWebView; v8 docs say iOS 15+ and Xcode 26+.[C1] | Actual UIKit/WKWebView implementation and device/Xcode archive instructions exist; current iOS guide says **experimental**.[W2,W4] |
| Maturity evidence | Dedicated mobile workflow and documented plugin APIs; this is evidence of an established supported path, not measured reliability for unbusy.day.[C1,C3,C5] | v3 is Beta; latest returned release `v3.0.0-beta.24` is prerelease. iOS is specifically experimental even though the general v3 label is beta.[W1,W2] |
| What runs locally | Bundled HTML/CSS/JS plus native host/plugins in the standard workflow.[C3] | Go compiled for iOS, UIKit bootstrap, assets served in-process via `wails://`, no loopback HTTP listener required.[W3,W4] |
| Native scope | App lifecycle and local-notification APIs are documented; add only needed plugins.[C5,C6] | Bindings/events, lifecycle, haptics, clipboard, share, secure storage, notifications and other mobile APIs are documented.[W2,W3] |
| Limitations relevant to iPad | Must validate our actual layout and host behavior; iOS platform support alone proves neither.[C1; recommendation] | Docs say only the first window is displayed, desktop window/menu/tray operations are no-ops, and save dialogs are unavailable; use sandbox plus share.[W2] |
| Tooling fit | Adds Node/native packaging alongside the Go-only server build; v8 requires Node 22+, macOS/full Xcode and recommends Swift Package Manager.[C1] | Also requires macOS/full Xcode and npm; current mobile guide says Go 1.25+. This is not a Node-free escape hatch.[W2,W3] |
| Repository fit | Best when kept a thin mobile presentation/native layer over the existing server. | More attractive if substantial Go work must run on-device; that is not the current requirement. |

**Recommendation:** pin a tested Capacitor v8 release and plugins rather than floating latest. Keep native build tooling separate from the Go deployment. If investigating Wails, pin beta.24 or a later explicitly evaluated release and require a real signed archive/TestFlight install before committing. The root `v3/IOS.md` still says Go 1.24+ while the current mobile guide says 1.25+; prefer the pinned module/toolchain requirements and `wails3 doctor`, not an assumption that every prose file is synchronized.[W2,W3]

### The Mac App Store is a separate question

Wails' macOS support and its Mac App Store guide concern a **macOS app**, not an iOS archive. Apple's Mac-specific rules include sandboxing; public-API rules also apply.[A1 §§2.4.5,2.5.1; W5] The Wails mobile example documents a `private_mac_apis` option for desktop translucency and a public-API fallback: do not carry that private-API configuration into a store release.[W3] The linked Mac packaging guide is not proof of v3 iOS approval or an automatically applicable v3 build recipe. Conversely, experimental iOS does not mean Wails cannot make a Mac app. Defer a separately designed/tested Mac target; it does not unblock iPhone/iPad launch.

## 3. The architecture decision that matters more than the framework

### A. Remote first-party server-rendered document

**Proposed flow:** WKWebView navigates to `https://unbusy.day/`; Go still renders the document/components, the session remains first-party, and existing relative requests go to the same server. A small native layer handles launch failures, lifecycle and the chosen native feature.

**Fit inference:** maximum reuse of current templ/Datastar/auth behavior and least frontend transport change. But every cold launch requires a functioning network/server; native recovery UI must work even if the page never loads. Server/CDN JavaScript and HTML patches become part of the native app's trust perimeter.

**Capacitor caveat:** `server.url` is a useful spike, not the documented production asset workflow. Simply adding `allowNavigation: ['*']` is neither a production design nor a security boundary.[C2] A production remote host would need deliberately maintained navigation, failure and bridge integration—potentially a custom native content view or a small Swift host rather than this configuration shortcut. **Unknown:** its maintenance cost and review acceptability for this product; neither has been tested.

### B. Bundled Capacitor web assets — preferred release candidate, subject to spike

Capacitor's workflow copies a built web directory into the native project. Its default iOS local scheme is `capacitor`, with hostname `localhost`; the iOS scheme cannot simply be changed to `https` to imitate the existing server origin.[C2,C3]

**Proposed flow:** bundle a minimal stable document, stylesheet, browser modules, Datastar and vendored CodeMirror; bootstrap authenticated view content from Fly, and keep server-rendered component patches as the UI read protocol. Introduce only the render/bootstrap and transport seams actually needed. Block invariants, Jotpad merging, habit dates and all durable writes remain Go services. Reuse `components.BlockColumn` for initial and patch rendering rather than inventing a parallel mobile block renderer.[R2]

This is **new engineering**, not “run `cap add ios`”:

1. Decide how a local document obtains its initial authenticated HTML/signals without navigating back onto the remote origin. Audit all `/static/...`, `/login`, form, module and SSE URLs; a local-root `/events` is not Fly's `/events`.
2. Decide who owns sessions. A local origin calling remote HTTPS is not the existing same-origin cookie context. Capacitor documents iOS third-party-cookie limitations and cookie facilities; that does **not** establish that a cookie plugin plus `credentials: include` preserves HttpOnly cookies, redirects, SSE and beacons automatically.[C4]
3. Prefer a narrow, explicitly tested native session/HTTP adapter if cross-origin cookie behavior cannot be made sound. Keep credentials out of JS; use secure native storage if a separate token is necessary. Provide a streaming read path, not a buffered “fetch this URL” plugin. Such an adapter is a transport change, not a replacement business service.[C7; recommendation]
4. Alternatively, if using direct web cross-origin requests, explicitly design allowed origins, credentials, preflight and CSRF defenses. Do not solve this by broadly allowing origins or weakening `SameSite` globally. Current middleware explicitly treats Lax as its baseline POST CSRF defense.[R3]
5. Preserve SSE cancellation/retry and the Jotpad's JSON acknowledgment semantics. Native HTTP monkey-patching must not silently break streaming or the teardown save path.
6. Remove/guard service-worker registration in the native shell as appropriate; do not rely on a PWA worker to make a custom-scheme app work. Bundle compatibility-sensitive UI resources so CDN failure cannot blank the installed shell.

**Decision gate:** choose B only if the spike demonstrates one shared component/read protocol, secure auth and real streaming without proliferating native special cases. Otherwise A with an explicitly owned host is the honest alternative. Wails' `wails://` origin would still need a corresponding remote transport/session seam; compiling Go locally does not magically preserve Fly's browser origin.[W3; inference]

### Native bridge trust boundary — applies to both

Apple requires public APIs and constrains downloaded code that introduces or changes app functionality. Its mini-app rules are not blanket permission for a remotely mutable native bridge.[A1 §§2.5.1,2.5.2,4.7] **Risk:** a remotely hosted script or a server-rendered Datastar expression can become native-capable code if given bridge access. Bundling JS does not remove this risk when streamed markup contains executable behavior.

**Required engineering controls (recommendation):**

- Expose only named operations such as “set this app's planning reminder,” with strict payload limits and native validation. No generic native method dispatcher, arbitrary filesystem paths, arbitrary network proxy, or secrets in web assets.
- Admit only the intended first-party main-frame content to privileged bridge calls; validate frame/origin and navigation at the native boundary, not merely by checking `location` in JS. Audit the exact pinned host implementation. Capacitor's source has navigation delegation and external-link handling, but that alone is not a complete bridge authorization audit.[C8]
- Open off-origin links outside the privileged content view. Test redirects, `target=_blank`, custom schemes, error pages, iframes and attempted navigation to attacker-controlled pages. Domain navigation controls are not a substitute for CSP or XSS protection.
- Treat templ-generated trusted UI and user-entered text differently. Keep blocks/Jotpad content escaped and inert; never promote arbitrary user HTML into Datastar expressions or native commands.
- Audit jsDelivr Datastar and Turnstile dependencies; self-host the pinned Datastar bundle where appropriate. Design CSP around actual Datastar expression evaluation and Turnstile needs, rather than claiming an untested strict CSP works. Keep third-party challenge content unprivileged.
- Use HTTPS, retain platform transport protections, ship no development server URL/cleartext exception, and keep mail credentials/database/admin secrets on Fly.[C7]
- Version the native/web contract; roll out compatible server changes and submit material feature changes through review. Do not use remote deploys to conceal unreviewed functionality.[A1 §§2.3.1,2.5.2; recommendation]

## 4. Login, account management and reviewer access

### Email OTP and Sign in with Apple

**Verified policy:** guideline 4.8's additional privacy-preserving login option requirement is triggered by third-party/social login for the primary account. It explicitly exempts an app exclusively using its own account setup/sign-in system.[A1 §4.8] **Application to this repository:** its own email OTP does not by itself require Sign in with Apple. SMTP and Turnstile are not a Google/Facebook identity login. Reassess if social login is introduced; Sign in with Apple is a practical qualifying option, not a feature required merely because the app is on iOS.

**Recommendation:** keep OTP in the app, preserve the persistent session, support paste/manual code entry, test expired/incorrect/resend/locked flows, and preserve state when the user switches to Mail and returns. Do not assume Safari and the installed app share a session. Explicitly handle the existing 401 responses from `/events` and mutation endpoints; do not spin endlessly or send login HTML to the SSE decoder.[R3]

The login requirement should be explained by private persistent data and cross-device synchronization. Apple asks apps without significant account-based features to work without sign-in.[A1 §5.1.1(v)] A meaningful anonymous Guide/demo is useful onboarding, but is not a substitute for reviewing authenticated features.

### Reviewer access is currently a gap

Apple asks for full access, an active demo account or suitable full-featured demo mode, a live backend and review instructions. App Store Connect says demo sign-in must not expire.[A1 Before You Submit, §2.1; A10] A normal 10-minute emailed code pasted into Review Notes will expire; a reviewer cannot be expected to coordinate with the developer's inbox.

**Recommendation:** before external TestFlight, choose and test a secure access mechanism: a dedicated synthetic review account with stable review credentials/recovery access, or an explicitly documented full-featured review mode agreed with Apple when appropriate. Do not assume a reduced static demo satisfies §2.1. A scoped review login would be new work: isolate it to synthetic data, rate-limit it, store no master credential in the binary, never create a universal OTP bypass, and disclose the mechanism to review. Test from a clean install without the maker's session or real email access. Keep it usable across review/re-review and concurrent reviewer sessions. Provide exact steps to reminders, Plan edits, Jotpad, Habits and deletion, plus a reachable reviewer contact. Account deletion must not irrecoverably strand the only review login.

### Account deletion is a submission blocker

Apple requires an in-app way to initiate deletion when account creation is supported, including automatic creation. Full account/data deletion—not logout or temporary deactivation—is required, subject to legally required retention. A direct web completion link can be used; ordinary apps should not force an email/phone support request. Reauthentication is allowed if not unnecessarily obstructive.[A3]

**Repository gap:** there is no deletion route. The current schema includes user email/bounds/Jotpad, sessions, blocks, `jot_base`, habits/check-ins, email-keyed login codes and a separate suppression table. Some tables cascade from the user, but suppression does not; login-code rows can also have a nullable user reference.[R3,R4]

**Recommended acceptance contract:**

- Settings → Delete account → clear consequences → confirmation/recent OTP if needed; execute server-side in the authenticated owner scope.
- Delete associated active records, explicitly handle email-keyed pending codes, invalidate all sessions and ensure already-open SSE connections cease exposing the deleted account. Test another signed-in device, not just the current cookie.
- Clear local drafts/caches/native credentials and cancel reminders on deletion/logout as appropriate; never show the prior user's draft after a new login.
- Document completion timing, backup expiration and legally justified retention. Audit Fly snapshots and restore procedures so a restore does not silently resurrect deleted accounts. Decide how minimal bounce/complaint suppression records are retained and disclosed; do not call them automatically deleted or legally exempt without analysis.
- If Sign in with Apple is later added, deletion must also revoke its tokens.[A3]

## 5. Reliability and current iOS compatibility

### SSE, background and offline are separate concerns

Apple documents that apps normally enter suspension shortly after moving to the background; limited additional execution time is not indefinite background runtime.[A8] Capacitor exposes native app-state/pause/resume events.[C5] **Therefore:** a 25-second server heartbeat is not an iOS background entitlement, and neither framework should promise continuously live background SSE.

**Recommended behavior:**

- Treat foreground SSE as disposable. On resume/network recovery, establish exactly one stream and obtain the authoritative snapshot; re-read the selected habit week using the existing guards. Cancel stale requests/listeners and refresh time-sensitive UI.
- Preserve unsaved Jotpad state during reconnect; do not reload the whole document as the default recovery if it would discard typing. Current retry/beacon logic is best effort, not durable process-death protection.[R2]
- Explicitly distinguish offline, reconnecting, session expired, server failure and saved. A network-status event is only a hint; a successful server response is the useful evidence.
- For launch, **do not queue offline block/habit mutations**. Disable unsupported writes and restore authoritative state after interrupted optimistic gestures. If keeping Jotpad typing enabled during outages, add a durable, account-scoped recovery draft with version/base context and a tested reconciliation flow. Otherwise make disconnected editing clearly unavailable; never silently imply it saved.
- Provide a bundled launch/error screen with Retry and Support on airplane-mode cold start. Optional read-only last-known data must be labeled stale and cleared on logout/deletion. Offline editing is not a stated blanket Apple requirement; a broken/blank experience is still a completeness risk.[A1 §2.1]
- Do not abuse audio/location/background modes just to keep SSE alive.[A1 §2.5.4]

### Compatibility evidence and the device test matrix

Both proposed hosts use **WKWebView** on iOS; switching from Capacitor to Wails does not replace its browser engine.[C1,W2] Capacitor's iOS 15 runtime minimum is **not** proof that this repository's CSS and JS support iOS 15.[C1]

| Dependency | Evidence and launch action |
| --- | --- |
| `command` / `commandfor` dialog buttons | WebKit explicitly documents these in Safari 26.2.[B1] The repo has feature-detected fallbacks; exercise both native and fallback paths. |
| `closedby="any"` | The repo supplies a `closedBy` fallback and comments “26.2.” The fetched 26.2 feature/release notes did **not** establish that precise introduction version. Treat the comment as unverified; test feature detection, backdrop dismissal and focus restoration.[R5,B1,B3] |
| `@scope`, nesting, layered CSS | Contemporary WebKit notes discuss `@scope` support/fixes, including nested declarations; this is not a verified oldest-supported-version matrix.[B1,B2] Because most component styles depend on it, inspect actual computed styles on the chosen minimum OS, not just whether the page loads. |
| Pointer capture, drag/stretch, touch scrolling | Repo behavior includes `preventDefault`, capture and `pointercancel` handling.[R5] Test dragging versus page scroll, interruption by native gestures, edges/home indicator, simultaneous SSE patches, touch targets and reduced motion. No native-host guarantee was established. |
| CodeMirror/contenteditable, keyboards | Test caret/selection, composition/IME, dictation, paste, autocorrect, undo, long notes, tab switching, keyboard dismissal and returning from Mail. Include iPad floating/split keyboard and hardware keyboard. Existing Android-specific handling is not evidence of iOS correctness.[R5] |
| Safe areas and viewport | WebKit documents `viewport-fit=cover` and `env(safe-area-inset-*)` for edge-to-edge content.[B4] Decide whether the host or CSS owns insets; avoid double padding. Current viewport markup does not request cover. Test notch/home indicator, landscape and keyboard resizing. |
| iPad layout | Test portrait/landscape and resizable/multitasking widths, not just a scaled phone screenshot. If considering Wails, account for its documented single-visible-window limitation.[W2] |
| Service worker, storage and auth | Verify WKWebView behavior independently of Safari/PWA; especially clean install, process termination, upgrade, logout, expiry and origin transitions. The existing passthrough worker does not supply offline assets.[R2] |

**Recommended provisional minimum:** evaluate iOS/iPadOS **26.2+** for the first small launch to limit compatibility work, then decide reach versus effort with the owner. This is a product/testing choice, **not Apple's required minimum deployment target**, not the earliest known support for all APIs, and not a claim that 26.2 fixes everything. Build-SDK requirements below are a separate concept. Test the minimum chosen OS and the current shipping OS on real iPhone and iPad hardware before declaring support.

Accessibility acceptance should include VoiceOver names/order, a non-drag way to move/resize blocks, large text/zoom without hidden controls, contrast across themes and reduced motion. These are recommended product gates; the existing Chromium smoke gate is not a WKWebView device test.[R2,R5]

## 6. Minimum functionality and a lean native scope

**Verified policy:** §4.2 says features/content/UI must elevate the app beyond a repackaged website and provide adequate lasting utility. §4.2.2 excludes primarily marketing/link collections. §4.2.7(e)'s thin-cloud-client language is within the Remote Desktop Clients section; do not misquote it as a blanket prohibition on every app with a remote backend.[A1]

**Approval risk:** a launch icon opening exactly the website, plus generic splash/haptics, is a weak submission story. The planner already offers interactive utility, but Apple alone decides whether the complete native experience is sufficient. Bundled assets improve engineering control; they do not establish compliance with 4.2.

**Recommended v1:**

1. Existing Plan, Jotpad and Habits, with genuinely usable phone/tablet layouts and reliable save/resume.
2. Native settings/recovery surface with privacy/support, session controls and deletion.
3. **Optional daily planning reminder**, chosen by the user and scheduled locally. Capacitor's Local Notifications API schedules on-device rather than requiring server push.[C6] Ask permission when enabled, handle denial, allow cancellation, use non-sensitive notification copy and test timezone/DST/time changes, app termination and notification taps.
4. Optional subtle haptic confirmation, only as polish. No tracking/ads or unrelated permission prompts.

Prefer a daily reminder over “notify at every block start” in v1: the rolling plan has no calendar date and other clients can change it while this device is suspended. Block alarms would need date/timezone semantics and stale-schedule reconciliation; background SSE cannot keep them universally current.[R1,A8; inference]

Defer widgets, Live Activities, Apple Watch, share extensions, Calendar/Health access, background push infrastructure, offline multi-writer editing and monetization. These are useful possible products, not prerequisites or approval insurance. If review rejects under 4.2, respond with a concrete demonstration of utility and improve the actual experience; do not just add permission prompts or rename the wrapper.

## 7. Privacy, data handling and payments

### Three separate privacy deliverables

1. **Privacy policy:** Apple requires a working link both inside the app and in App Store Connect, describing collection, uses, sharing and retention/deletion.[A1 §5.1.1(i)] Publish it and support access without requiring login. Cover the hosted service as well as the native app.
2. **App Privacy answers:** Apple requires disclosure of data collected by the app/developer and integrated third-party partners; data used only for functionality is still reportable, and account-linked data is generally linked to the user.[A4] This app is **not “Data Not Collected”** merely because Go stores the data on Fly.
3. **Privacy manifests / required-reason APIs:** Apple lists **Capacitor** among SDKs requiring manifests in applicable new submissions; listed SDKs used as binary dependencies also require signatures. APIs on the required-reason list need accurate approved reasons in the appropriate app/SDK manifest. These are distinct from the public privacy label.[A5,A7] Audit the final native archive, not just `package.json`.

**Provisional data inventory, to be finalized against production configuration:**

| Data / processing | Likely disclosure work, not a prefilled legal answer |
| --- | --- |
| Email address and user identifier | Contact Info → Email Address; Identifiers → User ID; linked to account, used for app functionality/authentication. Verify all actual uses.[A4,R3,R4] |
| Block labels, Jotpad text, habit names/check-ins | Assess User Content → Other User Content and any other applicable categories; these are persisted account-linked content, not merely transient local text.[A4,R4] |
| Auth/suppression, request/IP/security and mail logs | Audit what is actually retained, why, recipients and retention. Do not assume that “security” makes collection exempt.[A4,R3,R4] |
| Fly hosting/backups, SMTP provider, optional SES feedback, Turnstile, jsDelivr | Inventory the deployed vendors and actual data flows. Code proves integration points, not processor contracts, retention periods, tracking status or production settings.[R2,R3] |
| Native reminder time, possible recovery drafts, diagnostics | Disclose any transmitted data; decide local storage/access protection, removal and whether crash/analytics SDKs add collection. Prefer no new analytics SDK for v1.[A4; recommendation] |

“No tracking” may be the right final answer, but must follow this audit. Apple's tracking definition concerns cross-company advertising/measurement linkage or data brokers; ordinary authentication is not automatically tracking. ATT is required if tracking as defined is performed, not simply because an app makes network requests.[A4; A1 §5.1.2(i)]

Capacitor specifically calls out Preferences/UserDefaults and Filesystem as possible required-reason API consumers; its example reason code is not permission to paste a declaration unrelated to actual use.[C9] Check direct code, transitive plugins and the runtime; use Xcode's aggregate privacy report and upload validation. Wails would require the same app-specific audit, including any Go/native code reaching covered APIs—absence from a named-SDK list would not exempt such API use.[A5,A7]

### Payments — only if paid digital features are introduced

**Recommended v1:** free access, no checkout links, subscriptions or paid digital unlocks. Do not create a billing project for a currently non-billing app.

If paid digital planner functionality is introduced, Apple's default in-app digital-unlock rule is IAP, with important category/storefront exceptions. Current guidelines distinguish United States external purchase links and other region/entitlement rules; the free-companion rule in §3.1.3(f) also has conditions. Do not assume unbusy.day is a “reader app,” that a web subscription automatically escapes IAP, or that external links are banned everywhere.[A1 §§3.1.1,3.1.1(a),3.1.3] Re-research the exact product/regions then. StoreKit purchase, restore and entitlement handling would be a new native/server boundary; authoritative entitlements should remain server-side. This note does not choose a monetization model or quote a universal commission rate.

## 8. Enrollment, build and submission logistics

**Dated requirement:** as fetched September 21, 2026, Apple's requirements page says that **since April 28, 2026**, uploads to App Store Connect must use **Xcode 26 or later with the iOS/iPadOS 26 SDK or later**.[A2] Do not follow an older Xcode 16/iOS 18 checklist. This is the build SDK, not a requirement that the app's deployment target be iOS 26. Recheck immediately before uploading.

- **Membership:** Apple Developer Program is **US$99 per membership year**, with regional pricing and possible eligible fee waivers. An individual uses their legal name as seller; organizations require a legal entity, binding authority and ordinarily a D-U-N-S number. Apple Account two-factor authentication is required.[A9] Decide individual versus organization before branding the seller identity.
- **Build access:** both frameworks require macOS/full Xcode for their iOS workflow.[C1,W2] Budget Mac/cloud-Mac access plus real iPhone/iPad testing; a Linux Go server build does not produce the App Store archive. Choose a Mac/OS supported by the actual Xcode release rather than freezing a macOS minimum from this note.
- **App identity/signing:** establish bundle ID, team, native project signing and the App Store Connect app record; archive and validate the real Release build. Keep capabilities minimal. Follow the framework's Xcode workflow, not a simulator `.app` as the upload artifact.[C3,W2]
- **TestFlight:** internal testing precedes a small external cohort. Apple requires the first external build to have TestFlight App Review approval; shipping afterwards still requires app submission/review.[A11] Beta approval is not final App Store approval.
- **Listing:** prepare app name/description/category, icon, truthful release notes and screenshots showing actual use. Use Apple's current screenshot size table for the declared iPhone and iPad support rather than assuming phone screenshots satisfy both.[A10,A12] Use fictional plan/note data.
- **URLs/contact:** publish working support and privacy pages. The Support URL must lead to actual contact information; privacy must also be easily accessible inside the app. Supply reviewer name/email/phone and exact access instructions.[A10; A1 §5.1.1]
- **Age rating:** complete the current questionnaire honestly, including relevant app capabilities. Apple updated its rating system/questions with a January 31, 2026 deadline; do not select a rating solely because “it's a planner.”[A2,A13]
- **Export compliance:** answer encryption questions for the final binary, including system HTTPS and any added crypto/security libraries. Determine exemption/documentation rather than equating “we didn't write encryption” with “no encryption.” Apple's guidance assigns that assessment to the developer. Set the relevant Info.plist declaration only after the assessment.[A14]
- **Regions:** choose initial storefronts and resolve any applicable business/legal disclosures. Apple's current requirements include EU DSA trader-status requirements; a hobby label is not by itself the determination.[A2]

## 9. Phased implementation and acceptance checklist

All gates below are **recommendations**. Open implementation issues later; this document does not authorize or implement them.

### Phase 0 — Owner decisions and production inventory

- [ ] Choose seller identity, fee/device budget, initial storefronts and whether both iPhone/iPad ship together.
- [ ] Confirm free v1, minimum OS hypothesis, daily-reminder scope and acceptable internet dependence.
- [ ] Inventory deployed processors/logs/backups, publish support/privacy drafts, define deletion/retention policy.
- [ ] Decide a stable, secure reviewer-access mechanism; no expiring OTP pasted into metadata.

**Exit:** a bounded launch scope and no ambiguous claim of offline capability or native approval certainty.

### Phase 1 — Time-boxed architecture spike (suggested budget: several focused development days)

- [ ] Pin Capacitor/plugins; make an Xcode project and install on physical iPhone/iPad.
- [ ] Exercise existing remote-origin site as the reuse baseline; mark any `server.url` build **development only**.
- [ ] Demonstrate the bundled-shell candidate with one real authenticated initial render, a continuous `/events` stream, a block mutation/SSE acknowledgment and Jotpad CAS JSON acknowledgment.
- [ ] Test production Turnstile/OTP, persistent session, restart, logout and 401; validate native/web cookie ownership without JS exposure of credentials.
- [ ] Test off-origin navigation/redirect/iframe and attempted bridge misuse; confirm only narrowly allowed native operations.
- [ ] Archive/validate an actual Release build and inspect manifest/dependency requirements.

**Exit:** choose bundled Capacitor or an explicitly owned remote-content host based on measured complexity. Record bridge/auth/versioning decisions before expanding features. Stop rather than inventing an on-device SQLite replica or an insecure generic HTTP proxy. If considering Wails instead, require the same gates with the pinned beta; documentation alone does not pass them.

### Phase 2 — Complete the product, not just the wrapper

- [ ] Implement account deletion and verify all devices/sessions/streams, email-keyed rows, local drafts and reminders.
- [ ] Provide cold-start offline/server-error recovery and honest save state; no silent Jotpad loss in the supported recovery scenarios.
- [ ] Exercise clean install → OTP → create/move/stretch/rename/delete blocks → Jotpad save → habits → restart; verify persisted values from a second client.
- [ ] Background during a gesture/save, kill/relaunch, change network, expire session and restart Fly; reconnection leaves one active stream and converges without duplicate saves.
- [ ] Test phone/tablet orientation, resizing, keyboard/IME, dialog fallback/native paths, safe areas, VoiceOver and reduced motion on minimum/current OS.
- [ ] Daily reminder works with permission granted/denied, cancellation, timezone changes and notification taps; no implication it tracks every remote block change.
- [ ] Audit native permissions, privacy manifest/report, required-reason declarations and release logging; no user notes or session tokens in diagnostics.

**Exit:** no known data-loss, account-isolation, blank-launch or inaccessible-core-task defect; repeatable device test results and a signed build.

### Phase 3 — TestFlight

- [ ] Internal clean-install and upgrade testing, then a small external cohort using the actual review access path.[A11]
- [ ] Test on at least one physical iPhone and iPad, with minimum/current OS coverage where available; include poor connectivity and hardware keyboard.
- [ ] Collect usability/save/reconnect feedback with minimal personal data; resolve blockers rather than treating beta review as approval insurance.

**Exit:** testers can plan, write, check habits and return tomorrow without developer assistance; authentication/recovery and notification behavior are understood.

### Phase 4 — Submission

- [ ] Recheck current Apple guidelines, SDK requirements and the final dependency list.
- [ ] Complete screenshots, icon, support/privacy URLs, App Privacy, age rating, export compliance and regional disclosures.[A2,A4,A10,A12–A14]
- [ ] Keep Fly and mail/review access available; test review credentials from a clean install immediately before submission.
- [ ] Review Notes explain the planner's lasting utility, useful native integration, login/deletion steps, network dependence and all non-obvious features. Include a short demo attachment if useful, not as a substitute for working access.[A1 §2.1,A10]
- [ ] Submit; be available for questions. If rejected, address the cited actual issue and provide evidence. Do not promise a review timeline or acceptance.

## 10. Unresolved decisions / highest risks

1. **Host/origin architecture:** how much new transport does bundled HTML require, and is a small owned remote WKWebView host genuinely cheaper? Highest engineering uncertainty.
2. **4.2 judgment:** will this particular experience provide enough app-like utility? Native reminders are a hypothesis to validate, not a guarantee.
3. **Deployment floor:** is limiting v1 to 26.2+ acceptable? Older support requires a measured CSS/dialog/editor compatibility plan.
4. **Review login:** stable isolated credentials versus an agreed full-featured demo; neither exists today.
5. **Deletion/retention:** backup schedule, restore suppression and email-suppression treatment require operational and potentially legal decisions.
6. **Offline promise:** choose durable Jotpad draft recovery or clearly limited disconnected editing; current in-memory retries are insufficient for a durable guarantee.
7. **Production privacy inventory:** actual SMTP provider, Turnstile/CDN behavior, log retention, diagnostics and processor agreements were not verified.
8. **Native scope:** daily reminder timing/timezone semantics and account switching; avoid prematurely promising dated block alarms.
9. **Framework maintenance:** Capacitor/plugin pinned-release audit versus Wails experimental iOS; neither eliminates Apple review or WebKit testing.

## Primary-source bibliography and audit trail

All external sources below were fetched on **2026-09-21** unless marked blocked. Reference IDs above resolve to these exact URLs. Policy conclusions are limited to the cited sections; recommendations are the author's synthesis.

### Apple policy, operations and developer documentation

- **[A1]** App Review Guidelines — https://developer.apple.com/app-store/review/guidelines/ — HTTP 200; especially Before You Submit, §§2.1, 2.3.1, 2.4.5, 2.5.1–2.5.6, 3.1, 4.2, 4.7, 4.8, 5.1.
- **[A2]** Upcoming requirements — https://developer.apple.com/news/upcoming-requirements/ — 200; current SDK, age-rating and EU trader-status entries.
- **[A3]** Offering account deletion — https://developer.apple.com/support/offering-account-deletion-in-your-app/ — 200; initiation, complete deletion, web completion, reauthentication, retention, Sign in with Apple revocation.
- **[A4]** App Privacy details — https://developer.apple.com/app-store/app-privacy-details/ — 200; data collection, categories, linkage, tracking and optional-disclosure tests.
- **[A5]** Third-party SDK requirements — https://developer.apple.com/support/third-party-SDK-requirements/ — 200; named Capacitor entry, manifests/signatures and Xcode privacy report.
- **[A7]** Required-reason APIs — https://developer.apple.com/documentation/bundleresources/describing-use-of-required-reason-api — HTML shell only; actual documentation read via https://developer.apple.com/tutorials/data/documentation/bundleresources/describing-use-of-required-reason-api.json (200).
- **[A8]** Background execution — https://developer.apple.com/documentation/uikit/extending-your-app-s-background-execution-time — HTML shell only; content read via https://developer.apple.com/tutorials/data/documentation/uikit/extending-your-app-s-background-execution-time.json (200).
- **[A9]** Enrollment — https://developer.apple.com/programs/enroll/ — 200; legal identity, two-factor authentication, organization requirements and annual fee.
- **[A10]** Platform version information — https://developer.apple.com/help/app-store-connect/reference/app-information/platform-version-information/ — 200; screenshots, Support URL, reviewer contact, Notes and non-expiring demo sign-in. The attempted standalone https://developer.apple.com/help/app-store-connect/reference/app-information/app-review-information/ returned a generic shell, so claims use the populated platform-version page instead.
- **[A11]** TestFlight — https://developer.apple.com/testflight/ — 200; internal/external testing and separate final submission.
- **[A12]** Screenshot specifications — https://developer.apple.com/help/app-store-connect/reference/app-information/screenshot-specifications/ — 200; platform/device-specific current table.
- **[A13]** Age rating — https://developer.apple.com/help/app-store-connect/manage-app-information/set-an-app-age-rating/ — 200; current questionnaire workflow.
- **[A14]** Export compliance — https://developer.apple.com/help/app-store-connect/manage-app-information/overview-of-export-compliance/ — 200; encryption assessment and submission workflow.

### Capacitor — official documentation/source

- **[C1]** iOS — https://capacitorjs.com/docs/ios; environment — https://capacitorjs.com/docs/getting-started/environment-setup — both 200, v8. Environment page specifies SPM as the v8 default/recommendation despite older CocoaPods wording on the iOS overview.
- **[C2]** Configuration — https://capacitorjs.com/docs/config — 200; `webDir`, `server.url`, `allowNavigation`, `errorPath`, `hostname`, `iosScheme`.
- **[C3]** Workflow — https://capacitorjs.com/docs/basics/workflow — 200; local assets, sync and native tooling.
- **[C4]** Cookies — https://capacitorjs.com/docs/apis/cookies — 200; native cookie facilities and iOS third-party-cookie caveat. Not an end-to-end authentication proof.
- **[C5]** App lifecycle — https://capacitorjs.com/docs/apis/app — 200; app-state, pause/resume events.
- **[C6]** Local notifications — https://capacitorjs.com/docs/apis/local-notifications — 200; scheduling, permissions and notification events.
- **[C7]** Security guide — https://capacitorjs.com/docs/guides/security — 200; HTTPS, CSP, avoiding embedded secrets and secure credential storage.
- **[C8]** Native navigation implementation — https://raw.githubusercontent.com/ionic-team/capacitor/main/ios/Capacitor/Capacitor/WebViewDelegationHandler.swift — 200; inspected delegation/navigation logic. Moving main source, **not** a pinned release audit.
- **[C9]** Privacy manifest guide — https://capacitorjs.com/docs/ios/privacy-manifest — 200; plugin-specific implications and manifest example.

### Wails — official source, not an assumption about desktop-only support

- **[W1]** Root status — https://raw.githubusercontent.com/wailsapp/wails/master/README.md; release feed — https://api.github.com/repos/wailsapp/wails/releases?per_page=5 — both 200; v2 Stable/v3 Beta and beta.24 prerelease timestamp.
- **[W2]** Tagged iOS guide — https://raw.githubusercontent.com/wailsapp/wails/v3.0.0-beta.24/docs/mpress/content/guides/mobile/ios.md — 200; explicit experimental notice, requirements, device signing/Xcode archives and limitations. Same guide fetched at inspected commit: https://raw.githubusercontent.com/wailsapp/wails/4b1f348438d374e66892c9aeba7809a497f1edef/docs/mpress/content/guides/mobile/ios.md.
- **[W3]** Mobile overview — https://raw.githubusercontent.com/wailsapp/wails/4b1f348438d374e66892c9aeba7809a497f1edef/docs/mpress/content/guides/mobile/index.md; implementation notes — https://raw.githubusercontent.com/wailsapp/wails/master/v3/IOS.md; mobile example — https://raw.githubusercontent.com/wailsapp/wails/master/v3/examples/mobile/README.md — all 200. Overview confirms iOS **and Android**; notes detail custom-scheme transport; example documents native APIs/private-mac-API caveat.
- **[W4]** Tagged iOS implementation — https://raw.githubusercontent.com/wailsapp/wails/v3.0.0-beta.24/v3/pkg/application/application_ios.go — 200; `ios` build tag, Objective-C/cgo, UIKit/WebKit linkage and lifecycle implementation. Repository tree used for discovery: https://api.github.com/repos/wailsapp/wails/git/trees/master?recursive=1 (200). Source availability is not a successful local build.
- **[W5]** Mac App Store guide — https://raw.githubusercontent.com/wailsapp/wails/master/website/docs/guides/mac-appstore.mdx — 200; separate desktop packaging/sandbox guidance, not an iOS or pinned-v3 recipe.
- **Blocked:** https://v3.wails.io/ — HTTP 403; official repository docs above substituted, not secondary summaries.

### WebKit — first-party browser compatibility evidence

- **[B1]** Safari 26.2 features — requested https://webkit.org/blog/17640/webkit-features-in-safari-26-2/, redirected to https://webkit.org/blog/17640/webkit-features-for-safari-26-2/ — 200; button commands and `@scope` fixes.
- **[B2]** Safari 26 features — requested https://webkit.org/blog/17333/webkit-features-in-safari-26/, redirected to https://webkit.org/blog/17333/webkit-features-in-safari-26-0/ — 200; scoped CSS/nested declaration support and fixes. Also read https://webkit.org/blog/16574/webkit-features-in-safari-18-4/ and https://webkit.org/blog/16301/webkit-features-in-safari-18-2/ (200); these do not establish a complete minimum-OS compatibility contract.
- **[B3]** Safari 26.2 release notes — https://developer.apple.com/documentation/safari-release-notes/safari-26_2-release-notes — HTML shell; content read from https://developer.apple.com/tutorials/data/documentation/safari-release-notes/safari-26_2-release-notes.json (200). Exact `closedby` introduction not verified.
- **[B4]** Safe-area design — https://webkit.org/blog/7929/designing-websites-for-iphone-x/ — 200; historical first-party explanation of viewport-fit and safe-area variables, not a current device-test result.

### Local primary sources

- **[R1]** [CONTEXT.md](../../CONTEXT.md): rolling Day Plan, bounds, private Jotpad, habits, OTP and session vocabulary.
- **[R2]** [AGENTS.md](../../AGENTS.md), [main.go](../../cmd/unbusy/main.go), [router.go](../../cmd/unbusy/router.go), [layout.templ](../../internal/frontend/layouts/layout.templ), [events.go](../../internal/frontend/events.go), [Jotpad sync](../../internal/frontend/static/js/jot/sync.js), [service worker](../../internal/frontend/static/sw.js): architecture and runtime contracts.
- **[R3]** [Session middleware](../../internal/web/session.go), [auth service](../../internal/auth/auth.go), [auth wiring](../../cmd/unbusy/auth.go), [login markup](../../internal/frontend/components/login.templ), [routes](../../cmd/unbusy/router.go): session, OTP, provider integration and missing account-management routes.
- **[R4]** [Current schema](../../internal/migrate/schema.sql): persisted data and foreign-key relationships; deployment/backup intent from [AGENTS.md](../../AGENTS.md), not an audit of live infrastructure.
- **[R5]** [Pointer gestures](../../internal/frontend/static/js/blocks/pointer.js), [CodeMirror integration](../../internal/frontend/static/js/jot/cm.js), [dialog fallbacks](../../internal/frontend/static/js/invoker-fallback.js), [app.css](../../internal/frontend/static/css/app.css): actual browser-facing dependencies.

## Bottom line

**Capacitor first; Wails v3 mobile is real but experimental.** Preserve the Fly authority, prove the shell/auth/streaming boundary before investing in native features, and budget for deletion/privacy/reviewer access as first-class launch work. A useful, reliable native planning companion is the target—not a website icon plus a promise of approval.
