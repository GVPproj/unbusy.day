// Unit tests for now.js's pure time-math helpers — the now-slot and remaining-time
// logic that countdown.js and now-pill.js used to duplicate. The DOM glue (tick,
// the MutationObserver) isn't reachable here and is covered by /verify.
// Run: node --test internal/frontend/jstest
import test from "node:test";
import assert from "node:assert/strict";
import {
	secsSinceMidnight,
	nowSlot,
	slotFraction,
	remainingSecs,
	isActive,
	isPast,
	formatCountdown,
	formatClock,
} from "../static/now.js";

// A Date stand-in: only the local getters the helpers read.
const at = (h, m, s = 0) => ({ getHours: () => h, getMinutes: () => m, getSeconds: () => s });

test("secsSinceMidnight sums h/m/s into seconds from local midnight", () => {
	assert.equal(secsSinceMidnight(at(0, 0, 0)), 0);
	assert.equal(secsSinceMidnight(at(9, 30, 15)), 9 * 3600 + 30 * 60 + 15);
	assert.equal(secsSinceMidnight(at(23, 59, 59)), 86399);
});

test("nowSlot floors the clock into 30-min slots from midnight", () => {
	assert.equal(nowSlot(secsSinceMidnight(at(0, 0))), 0);
	assert.equal(nowSlot(secsSinceMidnight(at(0, 29, 59))), 0);
	assert.equal(nowSlot(secsSinceMidnight(at(0, 30))), 1);
	assert.equal(nowSlot(secsSinceMidnight(at(9, 0))), 18); // 9:00 → slot 18
	assert.equal(nowSlot(secsSinceMidnight(at(23, 30))), 47);
});

test("slotFraction is 0 at a slot boundary and approaches 1 at its end", () => {
	assert.equal(slotFraction(secsSinceMidnight(at(9, 0, 0))), 0);
	assert.equal(slotFraction(secsSinceMidnight(at(9, 15, 0))), 0.5); // 15 of 30 min
	assert.equal(slotFraction(secsSinceMidnight(at(9, 29, 59))), 1799 / 1800);
});

test("remainingSecs counts down to the block's exclusive end slot", () => {
	// Block ending at slot 20 (10:00); clock at 9:30:00 → 30 min left.
	assert.equal(remainingSecs(20, secsSinceMidnight(at(9, 30, 0))), 1800);
	assert.equal(remainingSecs(20, secsSinceMidnight(at(9, 59, 59))), 1);
	assert.equal(remainingSecs(20, secsSinceMidnight(at(10, 0, 0))), 0);
});

test("isActive is true only for the block spanning the current slot", () => {
	assert.equal(isActive(18, 2, 18), true); // [18,20) contains 18
	assert.equal(isActive(18, 2, 19), true);
	assert.equal(isActive(18, 2, 20), false); // end is exclusive
	assert.equal(isActive(18, 2, 17), false);
});

test("isPast is true once the slot passes the block's exclusive end", () => {
	assert.equal(isPast(18, 2, 20), true);
	assert.equal(isPast(18, 2, 19), false); // still active, not past
	assert.equal(isPast(18, 2, 25), true);
});

test("formatCountdown renders zero-padded HH:MM:SS", () => {
	assert.equal(formatCountdown(0), "00:00:00");
	assert.equal(formatCountdown(1800), "00:30:00");
	assert.equal(formatCountdown(3661), "01:01:01");
	assert.equal(formatCountdown(86399), "23:59:59");
});

test("formatClock renders 12-hour h:MM with no leading zero on the hour", () => {
	assert.equal(formatClock(0, 5), "12:05"); // midnight
	assert.equal(formatClock(9, 0), "9:00");
	assert.equal(formatClock(12, 30), "12:30"); // noon
	assert.equal(formatClock(13, 7), "1:07");
	assert.equal(formatClock(23, 59), "11:59");
});
