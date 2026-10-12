import { initializeApp, type FirebaseApp } from "firebase/app";
import { getAuth, GoogleAuthProvider, type Auth } from "firebase/auth";

const firebaseConfig = {
  apiKey: import.meta.env.VITE_FIREBASE_API_KEY,
  authDomain: import.meta.env.VITE_FIREBASE_AUTH_DOMAIN,
  projectId: import.meta.env.VITE_FIREBASE_PROJECT_ID,
  storageBucket: import.meta.env.VITE_FIREBASE_STORAGE_BUCKET,
  messagingSenderId: import.meta.env.VITE_FIREBASE_MESSAGING_SENDER_ID,
  appId: import.meta.env.VITE_FIREBASE_APP_ID,
  measurementId: import.meta.env.VITE_FIREBASE_MEASUREMENT_ID,
};

/**
 * Whether the build carries Firebase web credentials.
 *
 * Firebase sign-in is optional: the product authenticates with its own JWT and
 * sends one-time codes over WhatsApp, so a deployment without these values is a
 * valid configuration, not an error.
 */
export const isFirebaseEnabled = Boolean(
  firebaseConfig.apiKey && firebaseConfig.authDomain && firebaseConfig.projectId
);

let app: FirebaseApp | null = null;
let authInstance: Auth | null = null;
let provider: GoogleAuthProvider | null = null;

if (isFirebaseEnabled) {
  try {
    app = initializeApp(firebaseConfig);
    authInstance = getAuth(app);
    provider = new GoogleAuthProvider();
  } catch (err) {
    // A malformed key must not take the whole app down with it: getAuth throws
    // at module load, which previously blanked the page before React mounted.
    console.warn("Firebase sign-in disabled:", err);
    app = null;
    authInstance = null;
    provider = null;
  }
}

export const auth = authInstance;
export const googleProvider = provider;
