const VIM_PREFERENCE_KEY = "unbusy:jot-vim";

export function readVimPreference(storage) {
	try {
		return storage.getItem(VIM_PREFERENCE_KEY) === "true";
	} catch {
		return false;
	}
}

export function writeVimPreference(storage, enabled) {
	try {
		storage.setItem(VIM_PREFERENCE_KEY, enabled ? "true" : "false");
		return true;
	} catch {
		return false;
	}
}
