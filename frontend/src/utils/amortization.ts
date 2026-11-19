import type { TFunction } from "i18next";

/** Prestia's amortization systems. */
export type AmortizationType = "french" | "german" | "interest_included";

export const AMORTIZATION_TYPES: AmortizationType[] = ["french", "german", "interest_included"];

const SHORT: Record<AmortizationType, string> = {
  french: "loans.french",
  german: "loans.german",
  interest_included: "loans.interestIncluded",
};

const FULL: Record<AmortizationType, string> = {
  french: "loans.frenchFull",
  german: "loans.germanFull",
  interest_included: "loans.interestIncludedFull",
};

/** Short name of a system ("Francés", "Alemán", "Interés incluido"). */
export const amortizationLabel = (t: TFunction, type: string): string =>
  t(SHORT[type as AmortizationType] ?? "", { defaultValue: type });

/** Name with its rule, for selectors ("Francés (cuota fija)"). */
export const amortizationFullLabel = (t: TFunction, type: string): string =>
  t(FULL[type as AmortizationType] ?? "", { defaultValue: type });
