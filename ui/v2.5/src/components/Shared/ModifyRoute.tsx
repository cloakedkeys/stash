import React from "react";
import { Route, RouteProps } from "react-router-dom";
import { useCanModify } from "src/hooks/CurrentUser";
import { PageNotFound } from "../PageNotFound";

// A route to a page that only changes the library, for accounts allowed to do that.
export const ModifyRoute: React.FC<RouteProps> = (props) => {
  const canModify = useCanModify();

  if (!canModify) {
    return <Route {...props} component={PageNotFound} render={undefined} />;
  }

  return <Route {...props} />;
};
