# 006 — Proton Calendar outbound iCalendar subscription feed

Status: researched
Date: 2026-09-21

## Conclusion

**Yes.** Proton Calendar currently provides an outbound, refreshable iCalendar
feed through **Share with anyone → Create link**. It is not merely a browser
view: Proton tells recipients to put the URL into third-party calendars'
“subscribe to calendar” features, and its web client constructs an HTTPS URL
ending in `calendar.ics`. An external service such as unbusy.day can therefore
fetch that URL server-side and periodically parse the returned iCalendar data.

This is a read-only capability URL, not CalDAV and not an authenticated API. It
is available only for calendars created by users on a paid Proton plan, and the
owner currently has to create/manage it in the Proton Calendar web app. Proton
warns that updates can take up to eight hours, third-party subscribers refresh
on their own schedules, and some third-party subscriptions can stop updating
until the owner generates a new link. It is suitable for best-effort polling,
not real-time or SLA-backed synchronization.

## Keep the three mechanisms separate

| Direction/mechanism | What it does | Refreshes? | Writable? |
| --- | --- | --- | --- |
| **Outbound sharing link** (Proton → external consumer) | A Proton owner creates an iCalendar URL under **Share with anyone**. Google Calendar, Outlook, Apple Calendar, or another HTTP client can subscribe to it. | **Yes**, by repeatedly fetching the URL; Proton says changes may take up to eight hours to appear. | No; only the owner edits events. |
| **Inbound subscription** (external provider → Proton) | A Proton user adds somebody else's iCal (`.ics`) URL using **Other calendars → Add calendar from URL**. | Yes; Proton says it fetches third-party calendars every 4–16 hours. | No; subscribed calendars are view-only. |
| **One-time export/import** | The Proton owner downloads an ICS file, or a recipient opens the outbound sharing URL in a browser and downloads its current ICS response, then imports that file. | **No.** Proton explicitly says an imported downloaded file will not sync subsequent changes. | It is a snapshot, not a synchronization channel. |

Proton does not support CalDAV, so none of these provides direct two-way sync.

## Exact outbound sharing options

For a calendar the user created, Proton offers two materially different sharing
modes:

1. **Share with anyone via a link**
   - Anyone possessing the URL can view it; they do not need a Proton account.
   - The owner chooses **Limited view**, which exposes busy times without event
     details, or **Full view**, which exposes titles, descriptions,
     participants, and locations.
   - The calendar remains read-only to link holders.
   - The owner can create up to five links per calendar, label them to track
     audiences, and delete individual links. Different audiences can therefore
     have independently revocable URLs and different access levels.
2. **Share with Proton Mail users**
   - The owner invites specific Proton addresses, with **View** or **Edit**
     permission; an invitation must be accepted.
   - Up to 49 addresses can be entered per share operation.
   - Proton describes this as the more secure option because the calendar key is
     shared with named Proton users. It is not the route an unauthenticated
     unbusy.day poller can use.

Sharing is documented for **paid Proton plans** and must currently be initiated
from the **web app**, not the mobile apps. Calendars added/shared through the web
app remain visible in Proton's mobile apps. Proton's general calendar-management
page separately says Free users can create up to three personal calendars and
paid users up to 25; its feature table marks **Share calendar** unavailable on
Free and available on Paid.

## Is the generated link really an ICS subscription URL?

Yes, based on both Proton's support instructions and its official client code:

- Proton calls it an “iCal (.ics) link” and instructs recipients to use Google
  Calendar **From URL**, Outlook **Subscribe from web**, or Apple Calendar **New
  Calendar Subscription**.
- Opening the same URL in a browser downloads an ICS file. Importing that file is
  only a snapshot; preserving refresh behavior requires storing and refetching
  the URL as a subscription.
- At source commit
  [`05ee0bc`](https://github.com/ProtonMail/WebClients/commit/05ee0bc3cab8bf02c67bca4efdce511f80ca7f30),
  the web client builds links in this form:

  ```text
  https://calendar.proton.me/api/calendar/v1/url/{urlID}/calendar.ics?CacheKey={secret}
  https://calendar.proton.me/api/calendar/v1/url/{urlID}/calendar.ics?CacheKey={secret}&PassphraseKey={secret}
  ```

  The full-view form includes `PassphraseKey`; both forms include `CacheKey`.
  See
  [`shareUrl.ts`](https://github.com/ProtonMail/WebClients/blob/05ee0bc3cab8bf02c67bca4efdce511f80ca7f30/packages/shared/lib/calendar/sharing/shareUrl/shareUrl.ts#L119-L143)
  and the
  [`ACCESS_LEVEL` definition](https://github.com/ProtonMail/WebClients/blob/05ee0bc3cab8bf02c67bca4efdce511f80ca7f30/packages/shared/lib/interfaces/calendar/Link.ts#L1-L18).

This establishes a normal HTTPS resource returning iCalendar, consumable by a
server without running Proton's browser application. It does **not** establish a
publicly documented API contract or refresh SLA.

## Refresh behavior and operational caveats

- Proton says link subscribers see owner updates, but it may take **up to eight
  hours** before changes appear.
- Each third-party application chooses its own refresh schedule, so another
  delay can occur after subscribing.
- Proton documents a known issue in which Google, Outlook, Apple Calendar, or
  another third-party app can stop fetching a subscribed Proton link “after a
  period of time.” Its workaround is to generate a new link, unsubscribe the old
  URL, and subscribe to the new one, then delete the old link.
- Deleting a link stops calendars using it from syncing; deleting the source
  calendar deletes all its links. This revokes future access, not copies already
  downloaded by consumers.
- Proton publishes no fetch-frequency recommendation, conditional-request
  behavior, rate limit, availability guarantee, or maximum feed size on the
  cited outbound-sharing page. A poller should use a conservative interval,
  backoff, timeouts, response-size limits, and retain the last successful
  snapshot. The best interval should be validated against a real feed rather
  than inferred from Proton's end-to-end eight-hour warning.

Do not apply Proton's **4–16 hour** statement to the outbound feed: that interval
is specifically Proton's schedule when Proton is the inbound subscriber to a
third-party calendar.

## Privacy and security semantics

The sharing URL is a **bearer capability**: anyone who obtains it can retrieve
whatever its access level exposes. It must be handled like a secret, especially
because the key material is in the query string.

- **Limited view** minimizes disclosure to free/busy periods.
- **Full view** discloses event title, description, participants, and location.
  Proton says the URL contains the key needed to decrypt the calendar. When the
  URL is used, Proton temporarily has access to that key to decrypt the feed; at
  other times Proton says it cannot access the calendar. This is a deliberate
  exception to the normal claim that Proton cannot read end-to-end encrypted
  event details.
- Direct Proton-user sharing is stronger: keys are shared with individually
  selected users rather than anybody holding a URL.
- Link deletion is the revocation mechanism. Up to five independently labeled
  links make audience-specific rotation/revocation possible.

For unbusy.day, the full URL should therefore be encrypted at rest or otherwise
protected as credential material, excluded from logs/analytics/error reports,
and never placed in client-rendered markup. Administrative UI should show only a
redacted host/path. An importer should also prevent SSRF—for this integration,
accept only HTTPS URLs on `calendar.proton.me` matching Proton's calendar URL
path rather than fetching arbitrary user-supplied URLs.

## Suitability for unbusy.day server-side polling

**Suitable, with qualifications.** Route 2 can be implemented as a server-side,
read-only calendar import where the user pastes the generated **Share with
anyone** URL. No browser session, Proton login, or client-side decryption is
needed by the poller: possession of the complete capability URL is sufficient.

Product expectations should be explicit:

- require the user to have a paid Proton plan and create the link on the web;
- recommend Limited view unless event details are needed;
- label the integration “periodically refreshed,” not live sync;
- tolerate at least Proton's stated eight-hour lag;
- surface stale/error state and let the user replace a link if Proton's known
  stoppage occurs;
- treat deletion/HTTP failure as revoked or unavailable after retries, while
  preserving the last known snapshot according to product policy; and
- do not promise write-back, because the feed is read-only and Proton has no
  CalDAV support.

## Primary sources

All sources were accessed **2026-09-21**. Proton support articles do not display
publication or update dates in their rendered pages. Proton's [official
sitemap](https://proton.me/support/sitemap.xml) reported the support-page
modification dates listed below when accessed.

1. Proton Support, [How to share a calendar with anyone via a
   link](https://proton.me/support/share-calendar-via-link) — outbound modes,
   access levels, third-party subscription instructions, snapshot warning,
   five-link limit, revocation, eight-hour lag, known stoppage, and privacy
   warning. Sitemap last modified **2026-07-07**.
2. Proton Support, [Subscribe to an external
   calendar](https://proton.me/support/subscribe-to-external-calendar) — inbound
   subscription, read-only behavior, plan-calendar counting, 4–16 hour inbound
   refresh, iCalendar conformance, and lack of CalDAV. Sitemap last modified
   **2026-07-08**.
3. Proton Support, [How to share a calendar with Proton Mail
   users](https://proton.me/support/share-calendar-with-proton-users) — the
   account-scoped View/Edit alternative, invitation semantics, paid/web-only
   restriction, address limit, and key sharing. Sitemap last modified
   **2026-07-02**.
4. Proton Support, [How to create, edit, delete, and export calendars in Proton
   Calendar](https://proton.me/support/protoncalendar-calendars) — one-time
   **Download ICS** workflow and Free/Paid calendar limits. Sitemap last
   modified **2026-07-09**.
5. Proton Calendar, [Security and encryption
   explained](https://proton.me/calendar/security) — baseline end-to-end
   encryption claims for event details. No publication/update date was visible.
6. ProtonMail/WebClients, official open-source web client at commit
   [`05ee0bc`](https://github.com/ProtonMail/WebClients/tree/05ee0bc3cab8bf02c67bca4efdce511f80ca7f30)
   (commit date **2026-09-21**) — concrete `.ics` endpoint construction, query
   key material, and access-level types.
