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
import { ConsentPage, CookiesPage, PolicyPage, TermsPage } from "@/features/legal/LegalPage";
import { ThemeToggle } from "@/app/theme";
import { TeamLoginPage } from "@/features/auth/TeamLoginPage";
import ChatPage from "@/features/chat/ChatPage";
import TeamDeploymentsPage from "@/features/deployments/TeamDeploymentsPage";
import PlannerPage from "@/features/planner/PlannerPage";
import TeamPage from "@/features/team/TeamPage";
import CodingPage from "@/features/coding/CodingPage";
import AdminLoginPage from "@/features/administration/AdminLoginPage";
import AdministrationPage from "@/features/administration/AdministrationPage";
import { SamLayout } from "@/features/sam/SamLayout";
import SamDashboardPage from "@/features/sam/SamDashboardPage";
import SamCatalogPage from "@/features/sam/SamCatalogPage";
import SamLicensesPage from "@/features/sam/SamLicensesPage";
import SamInventoryPage from "@/features/sam/SamInventoryPage";
import SamCompliancePage from "@/features/sam/SamCompliancePage";
import SamProductPage from "@/features/sam/SamProductPage";
import SamHostPage from "@/features/sam/SamHostPage";
import SamMatchingPage from "@/features/sam/SamMatchingPage";
import SamAssistantPage from "@/features/sam/SamAssistantPage";
import { SiteConfigProvider } from "@/app/site-config";

function PublicNavigation() {
	return (
		<nav class="flex flex-wrap gap-x-5 gap-y-2" aria-label={m.mainNavigation()}>
			<a href="/">{m.footerHome()}</a>
			<A href="/questions">{m.questionsPageTitle()}</A>
			<A href="/planner">{m.plannerPageTitle()}</A>
			<A href="/sam">{m.samNavigation()}</A>
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
			try {
				return unwrap(await api.GET("/api/me"));
			} catch {
				return null;
			}
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
			location.pathname === "/questions");
	createEffect(() => {
		document.documentElement.toggleAttribute("data-mono", monoPage());
	});
	createEffect(() => {
		// The public site is one page: old question, plan and team URLs open it.
		if (
			location.pathname === "/questions" ||
			location.pathname === "/team"
		) {
			navigate("/", { replace: true });
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
									<A href="/">{m.publicSite()}</A>
								</header>
								{props.children}
							</div>
						}
					>
						<div class={shellClass()}>
							<a href="#main-content" class="sr-only focus:not-sr-only focus:absolute focus:z-50 focus:p-2 focus:bg-surface focus:text-text">
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
				<Route path="/" component={TeamPage} />
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
