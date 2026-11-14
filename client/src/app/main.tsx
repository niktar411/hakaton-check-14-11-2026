import {
	A,
	Route,
	Router,
	useLocation,
	useNavigate,
} from "@solidjs/router";
import {
	createEffect,
	createResource,
	createSignal,
	onMount,
	type JSX,
	Show,
} from "solid-js";
import { render } from "solid-js/web";
import "@/shared/styles/app.css";
import * as m from "@/paraglide/messages";
import { api, csrfHeaders, unwrap } from "@/shared/api";
import { Button } from "@/shared/ui/Button";
import { Footer } from "@/shared/ui/Footer";
import { LoadingState } from "@/shared/ui/State";
import { Toast } from "@/shared/ui/Toast";
import { DragonMark } from "@/shared/ui/DragonMark";
import { CookieBanner } from "@/shared/ui/CookieBanner";
import { ThemeToggle } from "@/app/theme";
import TeamPage from "@/features/team/TeamPage";
import { SiteConfigProvider } from "@/app/site-config";
import { lazyPage } from "@/app/lazyPage";

// Route components are code-split: every screen except the public landing
// is fetched on demand, so the first bundle stays small.
const TeamLoginPage = lazyPage(() => import("@/features/auth/TeamLoginPage"));
const ChatPage = lazyPage(() => import("@/features/chat/ChatPage"));
const TeamDeploymentsPage = lazyPage(() => import("@/features/deployments/TeamDeploymentsPage"));
const PlannerPage = lazyPage(() => import("@/features/planner/PlannerPage"));
const CodingPage = lazyPage(() => import("@/features/coding/CodingPage"));
const AdminLoginPage = lazyPage(() => import("@/features/administration/AdminLoginPage"));
const AdministrationPage = lazyPage(() => import("@/features/administration/AdministrationPage"));
const SamLayout = lazyPage(() =>
	import("@/features/sam/SamLayout").then((m) => ({ default: m.SamLayout })),
);
const SamDashboardPage = lazyPage(() => import("@/features/sam/SamDashboardPage"));
const SamCatalogPage = lazyPage(() => import("@/features/sam/SamCatalogPage"));
const SamLicensesPage = lazyPage(() => import("@/features/sam/SamLicensesPage"));
const SamInventoryPage = lazyPage(() => import("@/features/sam/SamInventoryPage"));
const SamCompliancePage = lazyPage(() => import("@/features/sam/SamCompliancePage"));
const SamProductPage = lazyPage(() => import("@/features/sam/SamProductPage"));
const SamHostPage = lazyPage(() => import("@/features/sam/SamHostPage"));
const SamMatchingPage = lazyPage(() => import("@/features/sam/SamMatchingPage"));
const SamAssistantPage = lazyPage(() => import("@/features/sam/SamAssistantPage"));
const TermsPage = lazyPage(() =>
	import("@/features/legal/LegalPage").then((m) => ({ default: m.TermsPage })),
);
const PolicyPage = lazyPage(() =>
	import("@/features/legal/LegalPage").then((m) => ({ default: m.PolicyPage })),
);
const ConsentPage = lazyPage(() =>
	import("@/features/legal/LegalPage").then((m) => ({ default: m.ConsentPage })),
);
const CookiesPage = lazyPage(() =>
	import("@/features/legal/LegalPage").then((m) => ({ default: m.CookiesPage })),
);

function PublicNavigation() {
	return (
		<nav class="flex flex-wrap gap-x-5 gap-y-2" aria-label={m.mainNavigation()}>
			<A href="/team">{m.teamPageTitle()}</A>
			<A href="/">{m.samNavigation()}</A>
		</nav>
	);
}

function TeamNavigation() {
	return (
		<nav class="flex flex-wrap gap-x-5 gap-y-2" aria-label={m.mainNavigation()}>
			<A href="/planner">{m.plannerPageTitle()}</A>
			<a href="/chat">{m.aiChatTitle()}</a>
			<A href="/deployments">{m.teamDeployments()}</A>
		</nav>
	);
}

function LogoutButton(props: { csrf?: string; onLoggedOut: () => void }) {
	const [pending, setPending] = createSignal(false);
	async function logout() {
		if (pending()) return;
		setPending(true);
		try {
			await api.POST("/api/logout", { headers: csrfHeaders(props.csrf) });
		} finally {
			setPending(false);
			props.onLoggedOut();
		}
	}
	return (
		<Button disabled={pending()} onClick={() => void logout()}>
			{m.logout()}
		</Button>
	);
}

function App(props: { children?: JSX.Element }) {
	const location = useLocation();
	const navigate = useNavigate();
	// Solid render appends instead of clearing the mount node, so the static
	// first-paint splash would stay on top of the page and block clicks.
	onMount(() => {
		for (const node of document.querySelectorAll("[data-boot]")) {
			node.remove();
		}
	});
	const [config, { refetch }] = createResource(async () =>
		unwrap(await api.GET("/api/public/site-config")),
	);
	const plannerHost = () => {
		if (window.location.hostname.startsWith("ai.")) return true;
		const base = config()?.plannerBaseUrl;
		if (!base) return window.location.hostname.startsWith("planner.");
		try {
			return window.location.hostname === new URL(base).hostname;
		} catch {
			return window.location.hostname.startsWith("planner.");
		}
	};
	// Start the session request immediately. Waiting for site-config first
	// added a full extra round trip before the login screen could render.
	const plannerHostname = () =>
		window.location.hostname.startsWith("planner.") ||
		window.location.hostname.startsWith("ai.");
	const [session, { refetch: refetchSession }] = createResource(
		// A real planner hostname can fetch the session immediately. A configured
		// custom host (localhost, a demo domain) becomes a planner host only
		// after site-config arrives, and the resource re-runs then. On the public
		// site the mode is "other", so /api/me is never requested.
		() => (plannerHostname() || plannerHost() ? "planner" : "other"),
		async (mode) => {
			if (mode !== "planner") return null;
			// A transient failure of /api/me must not flip the user to the
			// login screen: one short retry before giving up.
			for (let attempt = 0; attempt < 2; attempt++) {
				try {
					return unwrap(await api.GET("/api/me"));
				} catch {
					if (attempt === 0) {
						await new Promise((resolve) => setTimeout(resolve, 800));
						continue;
					}
					return null;
				}
			}
			return null;
		},
	);
	const gateLoading = () => plannerHost() && session.loading;
	const loginRequired = () =>
		plannerHost() && !session.loading && session() === null;
	const shellClass = () =>
		teamArea()
			? "flex min-h-dvh w-full flex-col"
			: "mx-auto flex min-h-dvh w-full max-w-6xl flex-col gap-6 p-4 md:p-8";
	const plannerArea = () =>
		location.pathname === "/planner" || location.pathname.startsWith("/planner/");
	const teamArea = () =>
		plannerArea() ||
		location.pathname === "/chat" ||
		location.pathname.startsWith("/chat/") ||
		location.pathname === "/deployments" ||
		location.pathname.startsWith("/deployments/");
	const adminArea = () =>
		location.pathname === "/admin" || location.pathname.startsWith("/admin/");
	const publicAdminLogin = () => location.pathname === "/admin/login";
	const privateArea = () =>
		adminArea() ||
		location.pathname === "/coding" ||
		location.pathname.startsWith("/coding/") ||
		location.pathname === "/planner" ||
		location.pathname.startsWith("/planner/");
	// The public team page is monochrome: the black-and-white palette keeps the
	// focus on the plan, the team and the assistant instead of decoration.
	const monoPage = () =>
		!plannerHost() &&
		(location.pathname === "/" ||
			location.pathname === "/team" ||
			location.pathname === "/questions" ||
			location.pathname === "/sam" ||
			location.pathname.startsWith("/sam/") ||
			location.pathname === "/terms" ||
			location.pathname === "/policy" ||
			location.pathname === "/consent" ||
			location.pathname === "/cookies");
	createEffect(() => {
		document.documentElement.toggleAttribute("data-mono", monoPage());
	});
	createEffect(() => {
		// The public site is one page: the legacy questions URL opens the
		// team page at /team (the root now hosts the SAM registry).
		if (location.pathname === "/questions") {
			navigate("/team", { replace: true });
		}
	});
	createEffect(() => {
		// The team host keeps its planner as the entry point.
		if (!config.loading && plannerHost() && location.pathname === "/") {
			navigate("/planner", { replace: true });
		}
	});
	createEffect(() => {
		const robots = document.querySelector('meta[name="robots"]');
		if (robots) {
			robots.setAttribute(
				"content",
				privateArea() ? "noindex,nofollow" : "index,follow",
			);
		}
	});
	return (
		<SiteConfigProvider
			value={{
				config,
				retry: () => {
					void refetch();
				},
			}}
		>
			<Toast />
			<CookieBanner />
			<Show
				when={!gateLoading()}
				fallback={
					<div class="flex min-h-dvh items-center justify-center p-4">
						<LoadingState />
					</div>
				}
			>
				<Show
					when={!loginRequired()}
					fallback={
						<TeamLoginPage onSuccess={() => void refetchSession()} />
					}
				>
					<Show
						when={!adminArea() || publicAdminLogin()}
						fallback={
							<div class="mx-auto flex min-h-dvh w-full max-w-7xl flex-col gap-6 p-4 md:p-8">
								<header class="border-border flex flex-wrap items-center justify-between gap-4 border-b pb-4">
									<A class="text-text text-xl font-bold no-underline" href="/admin">
										{m.administrationPageTitle()}
									</A>
									<A href="/team">{m.publicSite()}</A>
								</header>
								{props.children}
							</div>
						}
					>
						<div class={shellClass()}>
							<a href="#main-content" class="focus:bg-surface focus:text-text sr-only focus:not-sr-only focus:absolute focus:z-50 focus:p-2">
								{m.skipToContent()}
							</a>
							<header
								class={
									teamArea()
										? "flex flex-wrap items-center justify-between gap-4 px-4"
										: "border-border flex flex-wrap items-center justify-between gap-4 border-b px-4 pb-4"
								}
							>
								<A class="text-text inline-flex items-center gap-2 text-xl font-bold no-underline" href="/">
									<DragonMark class="size-8" />
									{m.appTitle()}
								</A>
								<div class="flex flex-wrap items-center gap-4">
									<Show when={plannerHost()} fallback={<PublicNavigation />}>
										<TeamNavigation />
									</Show>
									<ThemeToggle />
									<Show when={plannerHost()}>
										<LogoutButton
											csrf={(session() as { csrf?: string } | null)?.csrf}
											onLoggedOut={() => void refetchSession()}
										/>
									</Show>
								</div>
							</header>
							<main id="main-content" class="flex flex-1 flex-col">{props.children}</main>
							<Show when={!plannerHost()}>
								<Footer
									team={plannerHost()}
									borderless={plannerArea()}
								/>
							</Show>
						</div>
					</Show>
				</Show>
			</Show>
		</SiteConfigProvider>
	);
}

const root = document.getElementById("root");
if (root) {
	render(
		() => (
			<Router root={App}>
				<Route path="/" component={SamLayout}>
					<Route path="/" component={SamDashboardPage} />
					<Route path="/catalog" component={SamCatalogPage} />
					<Route path="/catalog/:id" component={SamProductPage} />
					<Route path="/licenses" component={SamLicensesPage} />
					<Route path="/inventory" component={SamInventoryPage} />
					<Route path="/inventory/:id" component={SamHostPage} />
					<Route path="/compliance" component={SamCompliancePage} />
					<Route path="/matching" component={SamMatchingPage} />
					<Route path="/assistant" component={SamAssistantPage} />
				</Route>
				<Route path="/team" component={TeamPage} />
				<Route path="/planner" component={PlannerPage} />
				<Route path="/chat" component={ChatPage} />
				<Route path="/deployments" component={TeamDeploymentsPage} />
				<Route path="/sam" component={SamLayout}>
					<Route path="/" component={SamDashboardPage} />
					<Route path="/catalog" component={SamCatalogPage} />
					<Route path="/catalog/:id" component={SamProductPage} />
					<Route path="/licenses" component={SamLicensesPage} />
					<Route path="/inventory" component={SamInventoryPage} />
					<Route path="/inventory/:id" component={SamHostPage} />
					<Route path="/compliance" component={SamCompliancePage} />
					<Route path="/matching" component={SamMatchingPage} />
					<Route path="/assistant" component={SamAssistantPage} />
				</Route>
				<Route path="/terms" component={TermsPage} />
				<Route path="/policy" component={PolicyPage} />
				<Route path="/consent" component={ConsentPage} />
				<Route path="/cookies" component={CookiesPage} />
				<Route path="/coding" component={CodingPage} />
				<Route path="/admin/login" component={AdminLoginPage} />
				<Route path="/admin" component={AdministrationPage} />
			</Router>
		),
		root,
	);
}
