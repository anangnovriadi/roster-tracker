import { useCallback } from "react";
import { useLocation, useNavigate } from "react-router-dom";
import { replaceFiltersByFactor, stringifyFilters, useNoteFilterContext } from "@/contexts/NoteFilterContext";

export const useDateFilterNavigation = () => {
  const navigate = useNavigate();
  const location = useLocation();
  const { filters, setFilters } = useNoteFilterContext();

  const navigateToDateFilter = useCallback(
    (date: string) => {
      const nextFilters = replaceFiltersByFactor(filters, "displayTime", [{ factor: "displayTime", value: date }]);
      const nextSearchParams = new URLSearchParams(location.search);
      nextSearchParams.set("filter", stringifyFilters(nextFilters));
      setFilters(nextFilters);
      navigate({ pathname: location.pathname, search: nextSearchParams.toString() });
    },
    [filters, location.pathname, location.search, navigate, setFilters],
  );

  return navigateToDateFilter;
};
