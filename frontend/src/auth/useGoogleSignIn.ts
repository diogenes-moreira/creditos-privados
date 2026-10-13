import { useCallback, useEffect, useRef, useState } from "react";

/**
 * Google Identity Services, loaded on demand.
 *
 * The client id ships in the bundle because it is public by design: it names
 * the application, it does not authorise anything. What proves identity is the
 * token Google signs, and the backend verifies that against Google's own keys.
 */
const GSI_SRC = "https://accounts.google.com/gsi/client";

const clientId = import.meta.env.VITE_GOOGLE_CLIENT_ID as string | undefined;

/** Whether this build can offer sign-in with Google. */
export const isGoogleSignInEnabled = Boolean(clientId);

interface CredentialResponse {
  credential?: string;
}

type GoogleAccounts = {
  accounts: {
    id: {
      initialize: (config: {
        client_id: string;
        callback: (response: CredentialResponse) => void;
      }) => void;
      renderButton: (parent: HTMLElement, options: Record<string, unknown>) => void;
    };
  };
};

declare global {
  interface Window {
    google?: GoogleAccounts;
  }
}

function loadScript(): Promise<void> {
  if (document.querySelector(`script[src="${GSI_SRC}"]`)) {
    return Promise.resolve();
  }
  return new Promise((resolve, reject) => {
    const script = document.createElement("script");
    script.src = GSI_SRC;
    script.async = true;
    script.onload = () => resolve();
    script.onerror = () => reject(new Error("Could not load Google sign-in"));
    document.head.appendChild(script);
  });
}

/**
 * Renders Google's own button into `ref` and hands the resulting ID token to
 * `onToken`. Google requires its rendered button rather than a custom one.
 */
export function useGoogleSignIn(onToken: (idToken: string) => void) {
  const ref = useRef<HTMLDivElement | null>(null);
  const [error, setError] = useState<string | null>(null);
  // Keep the latest callback without re-initializing the widget on each render.
  const handler = useRef(onToken);
  handler.current = onToken;

  const mount = useCallback(async () => {
    if (!clientId || !ref.current) return;
    try {
      await loadScript();
      const google = window.google;
      if (!google) throw new Error("Google sign-in unavailable");
      google.accounts.id.initialize({
        client_id: clientId,
        callback: (response) => {
          if (response.credential) handler.current(response.credential);
        },
      });
      google.accounts.id.renderButton(ref.current, {
        type: "standard",
        theme: "outline",
        size: "large",
        width: 320,
      });
    } catch (err) {
      setError(err instanceof Error ? err.message : "Google sign-in unavailable");
    }
  }, []);

  useEffect(() => {
    void mount();
  }, [mount]);

  return { ref, error };
}
