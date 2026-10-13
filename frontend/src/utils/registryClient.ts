import type { RegisterRequest, RegistryHolding, RegistryRancher } from "../api/types";

/**
 * Turns what the vaccination registry (RUV) knows about a rancher into a
 * pre-filled client registration.
 *
 * The registry was not built for onboarding, so part of this is inference and
 * the form has to say which part: `toReview` lists every field that was
 * guessed rather than read, and every field the registry does not hold at all
 * is left empty for the operator.
 */
export interface RegistryPrefill {
  form: Partial<RegisterRequest>;
  /** Fields that were filled by inference and must be checked by a person. */
  toReview: (keyof RegisterRequest)[];
}

/** Folds case, accents, spacing and a leading "La", as the backend does. */
export const normalizePlaceName = (name: string): string =>
  name
    .normalize("NFD")
    .replace(/[̀-ͯ]/g, "")
    .toLowerCase()
    .split(/\s+/)
    .filter(Boolean)
    .join(" ")
    .replace(/^la /, "");

/**
 * Finds the name in `candidates` the registry's spelling refers to: an exact
 * match once folded, or else the only candidate that contains it ("BOGOTA" in
 * "Bogotá D.C.", "SAN ANDRES" in the archipelago's full name). An ambiguous
 * prefix matches nothing, so the operator picks rather than inheriting a
 * wrong department.
 */
export const matchPlaceName = (registryName: string, candidates: string[]): string | undefined => {
  const wanted = normalizePlaceName(registryName);
  if (!wanted) return undefined;
  const exact = candidates.find((c) => normalizePlaceName(c) === wanted);
  if (exact) return exact;
  const partial = candidates.filter((c) => normalizePlaceName(c).includes(wanted));
  return partial.length === 1 ? partial[0] : undefined;
};

const titleCase = (s: string): string =>
  s
    .toLowerCase()
    .split(/\s+/)
    .filter(Boolean)
    .map((w) => w.charAt(0).toUpperCase() + w.slice(1))
    .join(" ");

/**
 * Splits the registry's single name field into first and last names.
 *
 * Colombians usually carry two given names and two surnames, written given
 * names first. With four or more words the first two are taken as given
 * names; with three, one given name and two surnames; with two, one of each.
 * It is a guess either way, which is why both fields are flagged for review.
 */
export const splitFullName = (fullName: string): { firstName: string; lastName: string } => {
  const words = titleCase(fullName).split(" ").filter(Boolean);
  if (words.length === 0) return { firstName: "", lastName: "" };
  if (words.length === 1) return { firstName: words[0], lastName: "" };
  const given = words.length >= 4 ? 2 : 1;
  return { firstName: words.slice(0, given).join(" "), lastName: words.slice(given).join(" ") };
};

const NIT_WEIGHTS = [3, 7, 13, 17, 19, 23, 29, 37, 41, 43, 47, 53, 59, 67, 71];

/** The DIAN check digit, applied right to left over the NIT's base. */
export const nitCheckDigit = (base: string): number => {
  let sum = 0;
  for (let i = 0; i < base.length; i++) {
    sum += Number(base[base.length - 1 - i]) * NIT_WEIGHTS[i];
  }
  const remainder = sum % 11;
  return remainder >= 2 ? 11 - remainder : remainder;
};

/**
 * A natural person's NIT is their cédula followed by the check digit. Only
 * valid if the rancher is registered with the DIAN under that cédula, which
 * the RUV cannot confirm, so it is flagged for review against the RUT.
 */
export const nitFromCedula = (cedula: string): string => {
  const digits = cedula.replace(/\D/g, "");
  return digits ? `${digits}-${nitCheckDigit(digits)}` : "";
};

/**
 * Puts the Colombian country code on a national mobile number. The registry
 * stores ten-digit numbers without it, and WhatsApp needs the full form.
 */
export const colombianPhone = (phone: string): string => {
  const digits = phone.replace(/\D/g, "");
  if (!digits) return "";
  if (digits.length === 12 && digits.startsWith("57")) return `+${digits}`;
  if (digits.length === 10) return `+57${digits}`;
  return phone.trim();
};

/**
 * The holding the client's address is taken from: the one with the most head,
 * which is where the rancher's operation actually is.
 */
export const mainHolding = (holdings: RegistryHolding[]): RegistryHolding | undefined =>
  [...holdings].sort((a, b) => b.head - a.head || b.year - a.year || b.cycle - a.cycle)[0];

type GeoModule = typeof import("country-state-city");

/**
 * Builds the pre-filled registration. `geo` resolves the registry's place
 * names to the ones the location selector offers; without it the department
 * and municipality are left for the operator.
 */
export const prefillFromRegistry = (
  rancher: RegistryRancher,
  country: string,
  geo?: GeoModule
): RegistryPrefill => {
  const toReview: (keyof RegisterRequest)[] = [];
  const { firstName, lastName } = splitFullName(rancher.name || "");
  if (firstName) toReview.push("firstName");
  if (lastName) toReview.push("lastName");

  const cuit = nitFromCedula(rancher.nationalId);
  if (cuit) toReview.push("cuit");

  const holding = mainHolding(rancher.holdings);
  let province = "";
  let city = "";
  let address = "";
  if (holding) {
    address = [holding.name && `Predio ${titleCase(holding.name)}`, holding.village && `vereda ${titleCase(holding.village)}`]
      .filter(Boolean)
      .join(", ");
    if (address) toReview.push("address");

    const co = geo?.Country.getAllCountries().find((c) => c.name === country);
    if (geo && co) {
      const states = geo.State.getStatesOfCountry(co.isoCode);
      const stateName = matchPlaceName(holding.department, states.map((s) => s.name));
      const state = states.find((s) => s.name === stateName);
      if (state) {
        province = state.name;
        const cities = geo.City.getCitiesOfState(co.isoCode, state.isoCode).map((c) => c.name);
        // The city field accepts free text, so an unmatched municipality is
        // kept as the registry spells it rather than dropped.
        city = matchPlaceName(holding.municipality, cities) ?? titleCase(holding.municipality);
      }
    }
  }

  return {
    form: {
      email: rancher.email?.trim().toLowerCase() || "",
      firstName,
      lastName,
      dni: rancher.nationalId,
      cuit,
      phone: rancher.phone ? colombianPhone(rancher.phone) : "",
      address,
      country,
      province,
      city,
    },
    toReview,
  };
};
