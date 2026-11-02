import { useLayoutEffect, useRef } from "react";
import { useLocation } from "react-router-dom";

// Keep this mounted above the routes so every way of opening Settings remembers
// the previous page, while tab changes and default-tab redirects leave it alone.
export function useSettingsReturnLocation() {
  const location = useLocation();
  const returnLocation = useRef<ReturnType<typeof useLocation>>();

  useLayoutEffect(() => {
    if (!/^\/settings(?:\/|$)/.test(location.pathname)) {
      returnLocation.current = location;
    }
  }, [location]);

  return returnLocation.current;
}
