# 005 — Sharing unbusy.day with like-minded people

Status: research only; no external issues, messages, or posts created  
Date / web access and attempted-access date: **2026-09-16**  
Repository inspected: `f06987e`  
Question: Where might this hobby project find interested people, and how could a marketing-averse maker share it without turning it into a marketing job?

## Recommendation in one paragraph

**Start with a small public note in your own space, show it to one person who already cares about planning, then choose just one outside venue.** My first community hypothesis is people already trying Newport-style daily time blocking—not everyone interested in “productivity.” The strongest verified invitation is surprisingly direct: Newport's contact page explicitly welcomes pointers to **tools** at `interesting@calnewport.com`, while warning that replies are usually not possible.[N3] Reddit's most relevant candidate communities need a manual rules check before posting; their pages were inaccessible here.[R] Show HN has actual precedents for very similar projects, but its current rules also need checking.[H1–H5] None of these facts predicts adoption or endorsement. The objective is to make your work visible and have one honest conversation, not to stage a launch campaign.

## Evidence labels and limits

- **Verified:** directly read in the repository, an official page, or an author's original post retrieved through the platform's official API. Citations accompany factual claims.
- **Fit inference:** my interpretation of that evidence, not evidence that a community wants this particular app.
- **Experiment / hypothesis:** a proposed action or expected effect to test, not an established outcome.
- **Unknown:** not verified. In particular, a community name, an old surviving promotional post, or a relevant topic is **not permission to self-promote**.

A background research agent investigated repository sources and external pages using repository reads and HTTP fetches. Reddit (`www` and `old`, plus its help domains), Hacker News's web pages, and `mastodon.social/about` failed to connect. Google's returned search pages supplied no usable results. HN's official Firebase API did work: Algolia search was used only to discover post IDs, then the cited posts were fetched from that API.[H0] No authenticated communities were entered, no account created, and no production login or browser interaction tested. Thus the hosted login/Guide observations below establish returned markup, not an end-to-end usability check.[L]

## 1. What you can honestly invite people to try

**Suggested short description:** “A small, Cal Newport-inspired web planner for shaping and reshaping today's time blocks, with a scratchpad and habit check-ins.” This describes the current repository, not a promised roadmap.[C1–C4]

| Verified current scope | Consequence for sharing |
| --- | --- |
| One rolling “today,” not dated day-plan history; 30-minute slots; configurable hours restricted to **04:00–18:00**. Clearing the plan is an explicit action, not a documented automatic midnight reset.[C1,C2] | Say “plan today,” not “manage your calendar.” Warn evening/night planners before they invest time. |
| Drag/stretch rearrangement with non-overlap and displacement of other blocks; Focus, Admin, Break, Fixed labels. “Fixed” appointments can still move and be pushed.[C1,C3] | A short fictional-day demonstration is more informative than a feature list. Don't imply appointments are locked. |
| A persistent Jotpad and separate weekly habit check-ins; templates are future work.[C1,C4] | Neither the habit matrix nor the scratchpad constitutes a complete weekly/quarterly planning system. Don't advertise templates. |
| Hosted `/` redirected to `/login`; the page offers email one-time-code login and “What is it?” Guide markup, including a small interactive drag/stretch example. The app routes require sessions.[L,C3,C5] | Disclose email login. Explain that the Guide is a preview, not a full anonymous planner. Test its interaction yourself before directing people to it. |
| The service-worker decision explicitly excludes offline caching/mode.[C6] | Don't call it offline-first or local-only. User-owned/private plans are not an end-to-end-encryption claim. |
| The repository uses **FSL-1.1-Apache-2.0**, with a competing-use restriction and an Apache grant effective two years after each version is made available.[C7,F] | Call it **source available (FSL)**, not presently “open source.” Don't imply that all future versions become Apache together on one date. |

The README's developer quickstart exists, but that is not evidence of a polished consumer self-hosting experience.[C2] Avoid unverified promises about pricing forever, uptime, calendar integration, clinical benefits, productivity gains, or data-security guarantees. Use “inspired by,” never “official” or “endorsed.”

### Why the audience hypothesis is plausible—but narrower than “Newport fans”

Newport describes assigning work to time blocks and revising the plan as reality changes; he explicitly says his aim is not rigid obedience to a schedule.[N1,N2] **Fit inference:** the app's re-blocking interaction is a more faithful introduction than “optimize every minute” or “get twice as much done.”

There is direct, albeit **historical**, evidence of differing preferences among his readers. In the September 2015 comments on *Three Recent Daily Plans*, Robert asks about apps; Paul M. Parks describes using Outlook for easier adjustment; the site's “Study Hacks” author replies that he is a “big paper booster.”[N2] These are individual first-person statements, not a survey, current demand estimate, or permission to post there. **Fit inference:** approach people who already want a digital daily sheet; don't try to convert happy paper users.

Digital minimalism is not simply “use no technology”: Newport's essay emphasizes carefully choosing tools that serve valued purposes and removing low-value noise.[N4] **Hypothesis:** “Does this replace planning friction, or add another place to fiddle?” is a better question for that audience than “Want another productivity app?”

## 2. Prioritized venues: permission and fit are separate

Priority below combines likely relevance, emotional effort, and verified permission—not audience size. “Conditional” means **not cleared to post**.

### Priority 1 — Your own small public note, plus a personal invitation

**Concrete place:** your personal site/blog or an existing personal profile; link to `https://unbusy.day/` and optionally `https://github.com/GVPproj/unbusy.day`. If useful later, your own repository's GitHub Discussions can host a welcome/feedback thread: GitHub documents owner-enabled Discussions and pinned welcome posts; I did not establish that this repository has Discussions enabled.[G]

**Recommendation:** publish one screenshot and a few honest paragraphs. Separately ask one to three existing acquaintances who actually time-block whether they would like to look. Don't assume the user has a mailing list, following, or existing community membership. Don't create those as prerequisites.

**Permission:** you control editorial space on your own site. Any hosted profile still has its own rules; no blanket Mastodon/Bluesky/LinkedIn permission was established here. A personal acquaintance is not an invitation to repeatedly chase them. Ask once; make declining easy.

**Fit inference:** this is the lowest-pressure way to practice being visible. A GitHub feedback thread would be a destination, not a discovery strategy. The site note can also be the long-lived alternative to continually “being social.”

### Priority 2 — Newport's “Interesting Links” inbox

**Verified invitation:** his official contact page asks readers to send pointers to “articles, books, tools, case studies, etc.” to **`interesting@calnewport.com`**. It says he reads and appreciates messages but is not usually able to reply. It explicitly says not to send non-academic mail to his Georgetown address.[N3]

**Recommendation:** one brief, personal email: what you built, the particular practice it explores, a link, and “no reply needed.” No follow-up sequence, request for endorsement, or expectation of newsletter coverage. This is a permissioned pointer to one person, **not a broadcast to his readers**.

**Fit inference:** strongest verified topical invitation, but potentially emotionally harder than a small public post. Make it optional; a non-response must not become the experiment's failure condition. His blog comments are useful audience evidence, but I found no verified current self-promotion permission for them—don't drop a link into an old essay.[N2,N3]

### Priority 3 — One narrowly relevant Reddit community, after permission

**Conditional shortlist, in order:**

| Candidate rules page | Fit hypothesis | What must be verified before sharing |
| --- | --- | --- |
| [r/CalNewport](https://www.reddit.com/r/CalNewport/about/rules/) | Most direct discussion of the inspiration; ask about revising a disrupted day. | Whether the community is active and admits tool posts; self-promotion rules; flair, account-age/karma requirements, and pinned threads. |
| [r/timeblocking](https://www.reddit.com/r/timeblocking/about/rules/) | Practice-first audience; ask whether block pushing makes replanning clearer. | Same checks; do not assume the literal name means an open showcase. |
| [r/DeepWork](https://www.reddit.com/r/DeepWork/about/rules/) | People interested in protecting focused effort. | Whether software posts belong at all, and whether links are restricted to a designated thread. |
| [r/productivity](https://www.reddit.com/r/productivity/about/rules/) | Broader, less specific backup if a current tool-sharing thread explicitly invites makers. | Exact self-promotion restrictions and current pinned-thread instructions; relevance alone is insufficient. |
| [r/digitalminimalism](https://www.reddit.com/r/digitalminimalism/about/rules/) | Only if members explicitly want a digital replacement for an existing practice. | Whether promotion is prohibited; avoid dressing an app advertisement up as a philosophy discussion. |

**Verification result:** all five rules API requests failed to connect; web-rule retries for the first two and an `old.reddit.com` retry also failed. The table is a list of **candidates to inspect**, not confirmation of their activity, membership, precise rules, or permission.[R]

**Recommendation:** manually read the sidebar/rules, current pinned posts, and recent relevant discussions. If rules prohibit promotion, stop. If genuinely ambiguous, send one moderator question naming your affiliation and proposed post. If a specific showcase thread is required, use only that thread; if links need advance approval, wait. Don't rely on a supposed universal Reddit “10% rule,” assume “hobby” is exempt, or accumulate performative comments to earn a promotional turn. A useful substantive reply with an affiliated link is still self-promotion and needs the same check.

### Priority 4 — Show HN, as a bounded second experiment

**Verified precedent:** the official HN API contains maker-authored submissions for *Timeist 2.0* (2023), a minimalist single-day non-overlapping time blocker, and *DeeProductivity* (2025), explicitly described by its maker as Newport-style time blocking.[H1,H2] A separate 2022 maker post specifically describes avoiding registration as a design motivation.[H3] These prove that people have submitted closely related work—not that HN endorsed it or that it succeeded.

**Verified historical restrictions, not a complete current rule check:** in a 2022 reply, `dang` says a Show HN is invalid if people cannot try it out and points to the signup-page restriction. In a 2014 moderation reply, the same account warns against voting rings, new-account manipulation and sockpuppet comments, and excludes a not-yet-tryable landing page/beta signup.[H4,H5] **Current [Show HN rules](https://news.ycombinator.com/showhn.html) and [general guidelines](https://news.ycombinator.com/newsguidelines.html) were inaccessible.** Re-read them manually, including rules about generated text, before composing/submitting anything.[H6]

**Recommendation:** link something people can meaningfully inspect/try, use a plain “Show HN:” title if eligible, state you made it, and explain one interesting design choice. Disclose the email requirement. The hosted app is not merely a waitlist, but its authenticated planner and small Guide demo should not be assumed to satisfy every current Show HN expectation.[L,C3,C5] A more obvious no-account playground could be a later improvement, not a new project required before sharing elsewhere. Don't ask friends to vote, seed praise, repost repeatedly, or conceal a limitation.

**Fit inference:** suitable overlap between makers and time-blockers; more public exposure and potentially more technical discussion than the first experiment needs. Ask one concrete question and give yourself a short reply window, not a day refreshing the ranking.

### Priority 5 — DEV: teach a technical lesson, don't paste a launch blurb

**Verified rules:** DEV's content policy requires on-topic, substantial, high-quality content not primarily for promotion/backlinks; a post cannot merely link to the real article elsewhere.[D1] Its AI guidelines require disclosure and fact-checking for AI-assisted articles, say those articles should not promote a business/program/course or primarily build a personal brand, and prohibit AI-generated comments except specified translation/grammar/assistive uses.[D2]

**Recommendation:** only if you want to write it yourself, explain a real implementation trade-off—e.g. why the server validates a client-computed non-overlapping layout—and include the project as the worked example.[C8] It should remain worth reading after removing the app link. **Do not copy an AI-written promotional draft from this research into DEV; adding an AI disclosure does not override the promotion restriction.** Treat this as a separate, optional writing exercise, not a required launch channel.

**Fit inference:** better for finding fellow builders than confirming whether the planner helps someone spend their day. No claim that an individual tag or organizer has invited this project.

## 3. Tempting places to deprioritize

- **Lobsters:** its own guidelines explicitly list “personal productivity systems” as off-topic. They welcome author participation but reject a write-only promotional presence; the stated rule of thumb is self-promotion below one quarter of stories/comments.[B] **Decision:** no planner announcement. A genuinely computing-focused article could be considered only if it meets topicality and participation rules; don't manufacture participation to qualify.
- **Indie Hackers:** the founder's own description centers profitable businesses, entrepreneurship and making money independently.[I] **Fit inference:** probably the wrong emotional frame for this hobby goal. The attempted `/guidelines` URL returned 404; no project-sharing permission was established. Don't convert the hobby into a revenue narrative just to fit.
- **r/SideProject:** plausible maker-sharing candidate, but its rules endpoint was inaccessible.[R] **Fit inference:** a fallback after verification, not automatically permissioned by its name; perhaps more builder feedback than time-blocking feedback.
- **r/selfhosted / open-source-only showcases:** rules unverified here, and the current project is FSL source available with a developer quickstart, not an established turnkey self-hosting offering.[R,C2,C7,F] **Decision:** avoid presenting it as open source or “one-click self-hosted.” Don't use other projects' issue trackers to advertise it.
- **Digital-minimalism, paper-planning, or screen-reduction groups:** inferred overlap in values is not evidence they want another web app. Newport's own writing and reader discussion show the distinction between deliberate tool use and enthusiasm for paper.[N2,N4] **Decision:** only enter a current, welcome conversation about a digital need; “paper works better for me” is a valid result.
- **Product Hunt, new Discord/Facebook groups, mass “build in public” activity:** not cleared by this research; no current rules checked. **Recommendation:** skip for this experiment, not because they cannot work, but because joining several unfamiliar venues would add work without answering the small question. No posting calendar or daily social habit needed.

## 4. A humane exposure ladder

These are **proposals**, not psychological treatment or proven marketing techniques. Choose one rung that is mildly uncomfortable, not overwhelming; stopping there counts.

1. **Make a private artifact.** Draft 100–200 words and one screenshot with fictional tasks. Don't expose Jotpad text, email, or anyone else's schedule. No publication obligation yet.
2. **Let one known person see it.** “I made something I'm a little nervous about sharing. Would you like to look?” Ask permission before sending a task list for them to perform.
3. **Put a durable note in your own space.** This is the first small broadcast. Explain what it does, one limitation, and how to reply; no “launch day” required.
4. **Make one permissioned outward gesture.** Choose the Newport email **or** a moderator-approved community post—not both as homework.
5. **Try a wider public venue only if you want to.** A rules-checked Show HN or a self-authored technical article is optional. You have already put your work out there at rung 3.

**Working hypothesis:** showing an artifact and asking a specific question may feel easier than persuading strangers to adopt an identity or buy a promise. Replace “How do I market myself?” with “Can I let another person see this thing I cared enough to make?”

An alternative to social selling: write one useful, durable account of how to re-plan a disrupted day, show both a paper option and your app, publish it on your own site, and link it only when relevant and welcome. Hypothesis: you may prefer occasional thoughtful writing and asynchronous replies to ongoing social performance. No audience-growth claim is implied.

## 5. One small experiment, with a stopping point

**Question:** “Can I show this to a like-minded person, and learn whether rearranging today's blocks feels useful or fussy?”

**Suggested budget:** at most 90 minutes of preparation, one external venue, two 15-minute reply windows over the following week.

1. Manually check the hosted login and Guide; make sure your description matches what a stranger can actually reach. The current anonymous response is login plus preview, not the saved planner.[L,C5]
2. Capture a fictional day: Focus → Admin → Break; demonstrate one interruption and a rearrangement. Add a still image/text alternative rather than requiring video watching.
3. Publish your small personal note. If ready, share it in **one** venue whose current rules permit it. Don't post an app link while asking moderators whether app links are allowed.
4. Offer a tiny, optional action: “If you already time-block, try moving one block when your plan changes. What felt awkward?” If they don't want to log in, screenshot feedback is welcome too.
5. Check replies at the two planned times. Thank people; don't defend every choice, promise requested features, or request a testimonial. After a week, write down one observation and stop.

**Success you control:** one truthful public artifact, proper permission/affiliation disclosure, and no follow-up pressure. **Bonus success:** one concrete observation, one genuinely interested conversation, or one person voluntarily trying it again. No replies is inconclusive—not a verdict on the work. “I prefer paper” and “18:00 is too early for my day” are useful fit boundaries, not failures. Stars, votes, impressions and signups are not the experiment's scorecard.

## 6. Ready-to-adapt introductions

These are **drafting aids, not instructions to paste generated text where it is prohibited**. Rewrite in your own voice, remove anything that is not true of your experience, and check current venue rules—including AI rules. Feature statements are grounded in [C1–C7,L]. “Hobby project” and reluctance to market come from the user's stated brief. No invented personal success story, endorsement, or testimonial is included.

### A. Personal post / note to existing contacts — first choice

> I've been making a small hobby project, **unbusy.day**: a Cal Newport-inspired planner for shaping and reshaping today's time blocks, with a scratchpad and habit check-ins.
>
> I'm trying to get a little more comfortable sharing things I make. If you already time-block and prefer a browser to paper, I'd be glad to hear what you think. It has 30-minute slots, currently stops at 18:00, and saves one rolling day rather than a history. The saved planner needs an emailed login code; “What is it?” on the login page gives a small preview.
>
> https://unbusy.day/
>
> One question, if you feel like looking: when your day changes, does rearranging the blocks seem helpful or like more fiddling? No obligation to try it.

### B. Moderator permission request — not a public ad

> Hi—I'm the maker of unbusy.day, a hobby time-block planner inspired by Cal Newport. I wanted to check whether a short, clearly disclosed post with a screenshot and a question about re-planning would be welcome here. The saved planner requires email login. Is there an appropriate tools/showcase thread, or would you prefer I not share it? I won't post it without clarity on the rules. Thanks.

Use only if the written rules leave a real ambiguity. A prohibition is already an answer.

### C. r/CalNewport or r/timeblocking — only after rules/permission check

**Possible title:** “I made a small time-block planner; looking for feedback on re-planning a disrupted day”

> Disclosure: I made this hobby project. It's inspired by Newport's time-blocking approach, not an official or endorsed tool.
>
> unbusy.day gives you one rolling day of half-hour blocks that you can drag and stretch as plans change. There's also a scratchpad and habit check-ins. Current limits include a 04:00–18:00 planning window and no day-plan history; the saved planner uses email-code login.
>
> For people who already plan this way: does pushing other blocks out of the way match how you'd revise your day, or would it be confusing? [Attach fictional-day screenshot; include https://unbusy.day/ only where permitted.]
>
> If paper already works for you, I'm not trying to talk you out of it.

For r/DeepWork, change the question to protecting a focus block after an interruption, rather than copying the same post across communities. Do not publish all three.

### D. Newport's invited inbox — short, no ask for coverage

**Subject:** “A small time-blocking tool inspired by your daily plans”

> Hi Cal,
>
> Your writing about revising time-block plans inspired a hobby project I've been building: https://unbusy.day/. It is a small browser planner for rearranging today's blocks, with a persistent scratchpad and habit check-ins.
>
> I'm sending it as a tool pointer through your Interesting Links invitation, not a request for promotion. The saved planner requires an emailed login code; the login page's “What is it?” Guide contains a small preview. It is deliberately limited to one rolling day, not your full planning system.
>
> No reply needed. Thank you for sharing the ideas that inspired it.

### E. Show HN — composition notes and an adaptable introduction

**Possible title:** “Show HN: Unbusy.day – a rolling-day time-block planner”

> I made unbusy.day, a hobby planner inspired by Cal Newport's time-blocking approach. Dragging or stretching a block rearranges the surrounding blocks without overlap. It also has a persistent scratchpad and habit check-ins.
>
> It is deliberately limited: half-hour slots, hours between 04:00 and 18:00, and one rolling day rather than day-plan history. The hosted planner uses email-code login; the login page has a small Guide demo, not the full saved planner.
>
> The source is available under FSL-1.1-Apache-2.0: https://github.com/GVPproj/unbusy.day. I'm interested in whether the re-planning interaction feels clear when a day gets interrupted.

Before using this: check current HN eligibility and generated-text rules.[H6] Compose the actual submission and replies yourself; this is not a prewritten first comment to paste. Don't add an unsupported “no signup,” “open source,” “free forever,” or superiority claim. You can leave the technology discussion for people who ask.

## Sources and audit trail

**All external sources below were accessed or attempted on 2026-09-16.** An inaccessible URL is a recheck destination, not evidence of its contents. Local links refer to the inspected repository; historical post dates are evidence dates, not current rules dates.

### Current tool — primary repository sources

- **[C1]** [CONTEXT.md](../../CONTEXT.md): Day Plan, Block Type, Push, Jotpad, Habit, Template, Login Code.
- **[C2]** [README.md](../../README.md) and [layout.go](../../internal/block/layout.go): purpose, quickstart, slots, bounds constants.
- **[C3]** [guide.templ](../../internal/frontend/components/modals/guide.templ): visible labels, Guide/preview and interactions.
- **[C4]** [companion.templ](../../internal/frontend/components/companion.templ): Jotpad/Habits tabs.
- **[C5]** [router.go](../../cmd/unbusy/router.go): login endpoints and session-gated app routes.
- **[C6]** [ADR 0010](../adr/0010-passthrough-service-worker-ios-pwa.md): explicit absence of offline mode/cache.
- **[C7]** [LICENSE.md](../../LICENSE.md); public copy also fetched: https://raw.githubusercontent.com/GVPproj/unbusy.day/main/LICENSE.md.
- **[C8]** [ADR 0005](../adr/0005-client-computed-push-server-enforced-invariants.md): client Push / server validation seam.
- **[L]** https://unbusy.day/ → https://unbusy.day/login, HTTP 200 after redirect. Returned login/Guide markup inspected; no OTP sent or authenticated workflow tested.
- **[F]** https://fsl.software/ — license steward's description and per-version two-year conversion explanation. Claims about the project's exact grant come from its own license, not the steward's promotional rationale.

### Newport — official writing and contact page

- **[N1]** https://calnewport.com/deep-habits-the-importance-of-planning-every-minute-of-your-work-day/ — time-block method and revisions; HTTP 200.
- **[N2]** https://calnewport.com/deep-habits-three-recent-daily-plans/ — intentionality rather than rigidity; September 29, 2015 reader/author comments cited above; HTTP 200. Historical firsthand preferences only.
- **[N3]** https://calnewport.com/contact/ — exact Interesting Links invitation, response caveat, and academic-address restriction; HTTP 200.
- **[N4]** https://calnewport.com/on-digital-minimalism/ — deliberate selection of valuable digital tools; HTTP 200.

### Hacker News — originals retrieved via official API

- **[H0]** https://github.com/HackerNews/API — official API documentation; HTTP 200. Discovery queries (not authorities for rules): https://hn.algolia.com/api/v1/search?query=timeblocking&tags=show_hn and https://hn.algolia.com/api/v1/search?query=showhn.html%20signup&tags=comment,author_dang.
- **[H1]** https://news.ycombinator.com/item?id=34408609 — `corneliusventi`, January 17, 2023, *Show HN: I made a minimalist time blocking app 2.0*. Verified original text via https://hacker-news.firebaseio.com/v0/item/34408609.json (200).
- **[H2]** https://news.ycombinator.com/item?id=43925170 — `FlorinDobinciuc`, May 8, 2025, Newport-style planner introduction. Verified via https://hacker-news.firebaseio.com/v0/item/43925170.json (200).
- **[H3]** https://news.ycombinator.com/item?id=30144295 — `jagadeep`, January 31, 2022, registration-free time-blocker rationale. Verified via https://hacker-news.firebaseio.com/v0/item/30144295.json (200). No independent verification of these other apps' features or continued availability.
- **[H4]** https://news.ycombinator.com/item?id=32916940 — `dang`, September 20, 2022, tryability/signup restriction. Verified via https://hacker-news.firebaseio.com/v0/item/32916940.json (200).
- **[H5]** https://news.ycombinator.com/item?id=7985275 — `dang`, July 3, 2014, voting manipulation/sockpuppets and not-yet-tryable beta landing page. Verified via https://hacker-news.firebaseio.com/v0/item/7985275.json (200).
- **[H6]** https://news.ycombinator.com/showhn.html and https://news.ycombinator.com/newsguidelines.html — connection failures; current full rules **not verified**. Historical replies cannot establish all current requirements or a categorical ban on authenticated working apps.

### Other community/platform primary sources

- **[D1]** https://dev.to/terms — §11 Content Policy; HTTP 200.
- **[D2]** https://dev.to/guidelines-for-ai-assisted-articles-on-dev — disclosure, promotion, and comment restrictions; HTTP 200. Also checked https://dev.to/code-of-conduct (200).
- **[G]** https://docs.github.com/en/discussions/quickstart — enabling Discussions, welcome posts, and community guidelines; HTTP 200.
- **[B]** https://lobste.rs/about — Topicality and Self-promotion; HTTP 200.
- **[I]** https://www.indiehackers.com/about — founder's community description; reached through `/faq` redirect, HTTP 200. https://www.indiehackers.com/guidelines returned 404; https://www.indiehackers.com/terms was readable but did not establish a specific showcase permission.
- **[R] Reddit access failures:** `https://www.reddit.com/r/{name}/about/rules.json`, with each of `CalNewport`, `timeblocking`, `DeepWork`, `productivity`, `digitalminimalism`, `SideProject`, and `selfhosted`, was requested and failed to connect. Also failed: https://old.reddit.com/r/CalNewport/about/rules.json; the CalNewport/timeblocking web rules URLs in the table; https://support.reddithelp.com/hc/en-us/articles/360043504051-Spam and its `reddithelp.com` variant. **No exact subreddit self-promotion rule, posting threshold, flair requirement, or current showcase thread was verified.**

## Bottom line

**An invitation is enough; you do not need a persona, a funnel, or a campaign.** Make one honest thing public, choose one welcome place to mention it, and give yourself permission to stop. The useful next result is a human conversation—not evidence that you have become good at marketing.
