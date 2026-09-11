import test from "node:test";
import assert from "node:assert/strict";
import { waitForTransitions } from "./transitions.js";

function transition(property, finished = Promise.resolve()) {
	return { transitionProperty: property, finished };
}

function element(...animations) {
	return { getAnimations: () => animations };
}

test("completion waits for every owned property across elements", async () => {
	let finishTransform;
	let finishHeight;
	const transform = new Promise((resolve) => { finishTransform = resolve; });
	const height = new Promise((resolve) => { finishHeight = resolve; });
	let done = false;
	const waiting = waitForTransitions([
		element(transition("transform", transform), transition("opacity", new Promise(() => {}))),
		element(transition("height", height)),
	], ["transform", "height"]).then(() => { done = true; });

	await Promise.resolve();
	assert.equal(done, false);
	finishTransform();
	await Promise.resolve();
	assert.equal(done, false);
	finishHeight();
	await waiting;
	assert.equal(done, true);
});

test("no-op and zero-duration targets complete immediately", async () => {
	await waitForTransitions([element()], ["transform", "height"]);
});

test("a canceled transition cannot strand completion", async () => {
	await waitForTransitions([
		element(transition("transform", Promise.reject(new Error("canceled")))),
	], ["transform"]);
});

test("unrelated transitions are ignored", async () => {
	await waitForTransitions([
		element(transition("opacity", new Promise(() => {}))),
	], ["transform", "height"]);
});
