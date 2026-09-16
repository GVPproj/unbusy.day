import { defineConfig } from "@playwright/test";

export default defineConfig({
	testDir: "./internal/frontend/browsertest",
	forbidOnly: !!process.env.CI,
	retries: 0,
	reporter: [["list"], ["html", { open: "never" }]],
	use: {
		trace: "retain-on-failure",
		screenshot: "only-on-failure",
	},
});
