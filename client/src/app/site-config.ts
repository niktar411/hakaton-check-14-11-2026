import { createContext, useContext } from "solid-js";
import type { Resource } from "solid-js";
import type { components } from "@/shared/api/schema";

type SiteConfig = components["schemas"]["SiteConfig"];
export type SiteConfigState = {
	config: Resource<SiteConfig | undefined>;
	retry: () => void;
};

const SiteConfigContext = createContext<SiteConfigState>();

export function useSiteConfig() {
	const state = useContext(SiteConfigContext);
	if (!state) throw new Error("site-config.context_missing");
	return state;
}

export const SiteConfigProvider = SiteConfigContext.Provider;
