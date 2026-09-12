import { expect, test } from "@playwright/test";
import { baseURL, signIn } from "./session.js";

test.use({ viewport: { width: 1200, height: 800 } });

test("theme radios support keyboard selection, synchronize, and persist", async ({ context, page }) => {
	await signIn(context);
	await page.goto(baseURL, { waitUntil: "load" });

	const root = page.locator("html");
	const theme = page.locator("#theme-modal");
	await theme.evaluate((dialog) => dialog.showModal());

	const cozy = theme.getByRole("radio", { name: "Cozy", exact: true });
	const solarized = theme.getByRole("radio", { name: "Solarized", exact: true });
	const light = theme.getByRole("radio", { name: "Light", exact: true });
	await expect(cozy).toBeChecked();
	await expect(solarized).toBeChecked();
	await expect(light).toBeChecked();

	await cozy.focus();
	await page.keyboard.press("ArrowRight");
	const mono = theme.getByRole("radio", { name: "Mono", exact: true });
	await expect(mono).toBeChecked();
	await expect(mono).toBeFocused();
	await expect(mono.locator("..")).toHaveCSS("outline-style", "solid");
	await expect(mono.locator("..")).toHaveCSS("outline-width", "2px");
	await expect(root).toHaveAttribute("data-feeling", "mono");

	await solarized.focus();
	await page.keyboard.press("ArrowRight");
	await expect(theme.getByRole("radio", { name: "Nord", exact: true })).toBeChecked();
	await expect(root).toHaveAttribute("data-colorscheme", "nord");

	await light.focus();
	await page.keyboard.press("ArrowRight");
	await expect(theme.getByRole("radio", { name: "Dark", exact: true })).toBeChecked();
	await expect(root).toHaveAttribute("data-colormode", "dark");
	await theme.getByRole("button", { name: "Done", exact: true }).click();

	const guide = page.locator("#guide-modal");
	await guide.evaluate((dialog) => dialog.showModal());
	for (let step = 1; step < 4; step++) {
		await guide.getByRole("button", { name: "Next", exact: true }).click();
	}
	const guideNord = guide.getByRole("radio", { name: "Nord", exact: true });
	const guideDark = guide.getByRole("radio", { name: "Dark", exact: true });
	const guideMono = guide.getByRole("radio", { name: "Mono", exact: true });
	await expect(guideNord).toBeChecked();
	await expect(guideDark).toBeChecked();
	await expect(guideMono).toBeChecked();

	await guideNord.focus();
	await page.keyboard.press("ArrowRight");
	await expect(root).toHaveAttribute("data-colorscheme", "catppuccin");
	await guideDark.focus();
	await page.keyboard.press("ArrowRight");
	await expect(root).toHaveAttribute("data-colormode", "light");
	await guideMono.focus();
	await page.keyboard.press("ArrowRight");
	await expect(root).toHaveAttribute("data-feeling", "cozy");
	await guide.getByRole("button", { name: "Done", exact: true }).click();

	await theme.evaluate((dialog) => dialog.showModal());
	await expect(theme.getByRole("radio", { name: "Catppuccin", exact: true })).toBeChecked();
	await expect(theme.getByRole("radio", { name: "Light", exact: true })).toBeChecked();
	await expect(theme.getByRole("radio", { name: "Cozy", exact: true })).toBeChecked();
	await theme.getByRole("button", { name: "Done", exact: true }).click();

	await page.reload({ waitUntil: "load" });
	await expect(root).toHaveAttribute("data-colorscheme", "catppuccin");
	await expect(root).toHaveAttribute("data-colormode", "light");
	await expect(root).toHaveAttribute("data-feeling", "cozy");
	const reloadedTheme = page.locator("#theme-modal");
	await reloadedTheme.evaluate((dialog) => dialog.showModal());
	await expect(reloadedTheme.getByRole("radio", { name: "Catppuccin", exact: true })).toBeChecked();
	await expect(reloadedTheme.getByRole("radio", { name: "Light", exact: true })).toBeChecked();
	await expect(reloadedTheme.getByRole("radio", { name: "Cozy", exact: true })).toBeChecked();
});
