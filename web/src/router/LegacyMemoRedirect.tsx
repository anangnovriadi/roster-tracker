import { Navigate, useLocation } from "react-router-dom";
import { ROUTES } from "./routes";

/** Sends links from before notes moved from /memos/ to /notes/ to the same note. */
export const LegacyMemoRedirect = () => {
  const location = useLocation();
  const pathname = location.pathname.replace(/^\/memos\//, `${ROUTES.NOTE}/`);
  return <Navigate to={{ pathname, search: location.search, hash: location.hash }} replace />;
};
