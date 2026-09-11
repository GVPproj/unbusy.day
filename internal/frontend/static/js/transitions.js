// Wait for the CSS transitions currently owned by a gesture. Canceled
// transitions reject `finished`, so allSettled is the completion barrier.
export async function waitForTransitions(elements, properties) {
	const owned = new Set(properties);
	const animations = elements.flatMap((el) =>
		el.getAnimations().filter((animation) =>
			owned.has(animation.transitionProperty),
		),
	);
	await Promise.allSettled(animations.map((animation) => animation.finished));
}
