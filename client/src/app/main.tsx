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
import { ThemeToggle } from "@/app/theme";
import { TeamLoginPage } from "@/features/auth/TeamLoginPage";
import ChatPage from "@/features/chat/ChatPage";
import TeamDeploymentsPage from "@/features/deployments/TeamDeploymentsPage";
import PlannerPage from "@/features/planner/PlannerPage";
import TeamPage from "@/features/team/TeamPage";
import CodingPage from "@/features/coding/CodingPage";
import AdminLoginPage from "@/features/administration/AdminLoginPage";
import AdministrationPage from "@/features/administration/AdministrationPage";
import { SiteConfigProvider } from "@/app/site-config";

function PublicNavigation() {
	return (
		<nav class="flex flex-wrap gap-x-5 gap-y-2" aria-label={m.mainNavigation()}>
			<A href="/">{m.teamPageTitle()}</A>
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
							<header
								class={
									teamArea()
										? "flex flex-wrap items-center justify-between gap-4 px-4"
										: "border-border flex flex-wrap items-center justify-between gap-4 border-b px-4 pb-4"
								}
							>
								<A class="text-text inline-flex items-center gap-2 text-xl font-bold no-underline" href="/">
									<img class="size-8 rounded-full" src="/logo-192.png" alt="" />
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
							<div class="flex flex-1 flex-col">{props.children}</div>
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
				<Route path="/coding" component={CodingPage} />
				<Route path="/admin/login" component={AdminLoginPage} />
				<Route path="/admin" component={AdministrationPage} />
			</Router>
		),
		root,
	);
}
