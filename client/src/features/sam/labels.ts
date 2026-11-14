import * as m from "@/paraglide/messages";
import { formatMoney } from "@/shared/lib/format";
import type { SamTone } from "./SamPage";

export function riskCostLabel(value: number) {
	return value > 0 ? formatMoney(value) : m.samRiskCostUnknown();
}

export function productRiskCostLabel(product: {
	riskCost: number;
	overusedSeats: number;
	expiredSeats: number;
	idleSeats: number;
	bannedInstallations: number;
}) {
	if (product.riskCost > 0) return formatMoney(product.riskCost);
	const hasViolation =
		product.overusedSeats > 0 ||
		product.expiredSeats > 0 ||
		product.idleSeats > 0 ||
		product.bannedInstallations > 0;
	return hasViolation ? m.samRiskCostUnknown() : formatMoney(0);
}

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

export function productStatusLabel(value: string) {
	switch (value) {
		case "active":
			return m.samProductStatusActive();
		case "inactive":
			return m.samProductStatusInactive();
		case "archived":
			return m.samProductStatusArchived();
		case "banned":
			return m.samProductStatusBanned();
		default:
			return value;
	}
}

export function productStatusTone(value: string): SamTone {
	switch (value) {
		case "active":
			return "success";
		case "inactive":
			return "warning";
		case "archived":
			return "neutral";
		case "banned":
			return "danger";
		default:
			return "neutral";
	}
}

export function riskLevelLabel(value: string) {
	switch (value) {
		case "high":
			return m.samRiskLevelHigh();
		case "medium":
			return m.samRiskLevelMedium();
		case "low":
			return m.samRiskLevelLow();
		default:
			return value;
	}
}

export function riskLevelTone(value: string): SamTone {
	switch (value) {
		case "high":
			return "danger";
		case "medium":
			return "warning";
		case "low":
			return "success";
		default:
			return "neutral";
	}
}

export function riskCodeLabel(value: string) {
	switch (value) {
		case "banned_product":
			return m.samRiskCodeBanned();
		case "license_overuse":
			return m.samRiskCodeOveruse();
		case "expired_license":
			return m.samRiskCodeExpired();
		case "idle_license":
			return m.samRiskCodeIdle();
		case "inactive_product":
			return m.samRiskCodeInactive();
		case "unmatched_installations":
			return m.samRiskCodeUnmatched();
		default:
			return value;
	}
}

export function hostLicenseStatusLabel(value: string) {
	switch (value) {
		case "licensed":
			return m.samLicenseStatusLicensed();
		case "expired":
			return m.samLicenseStatusExpired();
		case "unlicensed":
			return m.samLicenseStatusUnlicensed();
		case "banned":
			return m.samLicenseStatusBanned();
		case "inactive":
			return m.samLicenseStatusInactive();
		default:
			return value;
	}
}

export function hostLicenseStatusTone(value: string): SamTone {
	switch (value) {
		case "licensed":
			return "success";
		case "expired":
			return "warning";
		case "unlicensed":
			return "danger";
		case "banned":
			return "danger";
		case "inactive":
			return "warning";
		default:
			return "neutral";
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
