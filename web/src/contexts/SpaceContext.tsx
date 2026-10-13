import { createContext, type ReactNode, useContext } from "react";

interface SpaceContextValue {
  /** The creator a collection is narrowed to. Roster Tracker always shows everyone's notes. */
  creatorUsername?: string;
  memoFilter?: string;
}

const SpaceContext = createContext<SpaceContextValue | null>(null);

// Notes are not browsed per member, so every collection shows everyone's notes.
const ALL_NOTES: SpaceContextValue = {};

export function SpaceProvider({ children }: { children: ReactNode }) {
  return <SpaceContext.Provider value={ALL_NOTES}>{children}</SpaceContext.Provider>;
}

export function useSpaceContext() {
  const context = useContext(SpaceContext);
  if (!context) throw new Error("useSpaceContext must be used within SpaceProvider");
  return context;
}
