import * as m from "@/paraglide/messages";
import type { SamTone } from "./SamPage";

export function licenseKindLabel(value: string) {
	switch (value) {
		case "subscription":
			return m.samKindSubscription();
		case "perpetual":
			return m.samKindPerpetual();
		case "trial":
			return m.samKindTrial();
		default:
			return value;
	}
}

export function licenseStatusLabel(value: string) {
	switch (value) {
		case "active":
			return m.samLicenseActive();
		case "expiring":
			return m.samLicenseExpiring();
		case "expired":
			return m.samLicenseExpired();
		default:
			return value;
	}
}

export function licenseStatusTone(value: string): SamTone {
	switch (value) {
		case "active":
			return "success";
		case "expiring":
			return "warning";
		case "expired":
			return "danger";
		default:
			return "neutral";
	}
}

export function findingSeverityLabel(value: string) {
	switch (value) {
		case "low":
			return m.samSeverityLow();
		case "medium":
			return m.samSeverityMedium();
		case "high":
			return m.samSeverityHigh();
		case "critical":
			return m.samSeverityCritical();
		default:
			return value;
	}
}

export function findingSeverityTone(value: string): SamTone {
	switch (value) {
		case "low":
			return "neutral";
		case "medium":
			return "info";
		case "high":
			return "warning";
		case "critical":
			return "danger";
		default:
			return "neutral";
	}
}

export function findingStatusLabel(value: string) {
	switch (value) {
		case "open":
			return m.samFindingOpen();
		case "acknowledged":
			return m.samFindingAcknowledged();
		case "resolved":
			return m.samFindingResolved();
		default:
			return value;
	}
}

export function matchStatusLabel(value: string) {
	switch (value) {
		case "suggested":
			return m.samMatchSuggested();
		case "confirmed":
			return m.samMatchConfirmed();
		case "rejected":
			return m.samMatchRejected();
		default:
			return value;
	}
}

export function matchStatusTone(value: string): SamTone {
	switch (value) {
		case "suggested":
			return "warning";
		case "confirmed":
			return "success";
		case "rejected":
			return "neutral";
		default:
			return "neutral";
	}
}

export function suggestionLabel(kind: string) {
	switch (kind) {
		case "expiring_licenses":
			return m.samSuggestionExpiring();
		case "open_findings":
			return m.samSuggestionFindings();
		case "pending_matches":
			return m.samSuggestionMatches();
		case "unlicensed_installations":
			return m.samSuggestionUnlicensed();
		default:
			return kind;
	}
}
