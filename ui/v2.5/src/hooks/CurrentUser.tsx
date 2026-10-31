import React from "react";
import * as GQL from "src/core/generated-graphql";

export type CurrentUser = NonNullable<GQL.MeQuery["me"]>;

export interface ICurrentUserContext {
  user: CurrentUser | null;
}

const CurrentUserContext = React.createContext<ICurrentUserContext>({
  user: null,
});

export const useCurrentUser = () => {
  return React.useContext(CurrentUserContext).user;
};

// Returns true if the current user is an administrator.
// Some fields, such as organized and ignore_auto_tag, may only be changed by administrators.
export const useIsAdmin = () => {
  const user = useCurrentUser();
  return user?.roles?.includes(GQL.RoleEnum.Admin) ?? false;
};

// The user is only null until it has loaded. These two allow it in that case, because
// they decide what to show: the server checks the role on every request anyway.
const useHasRole = (...roles: GQL.RoleEnum[]) => {
  const user = useCurrentUser();
  return !user || roles.some((role) => user.roles?.includes(role));
};

// Returns true if the current user may change the library.
export const useCanModify = () =>
  useHasRole(GQL.RoleEnum.Admin, GQL.RoleEnum.Modify);

// Returns true if the current user may use what is limited to administrators.
export const useCanAdminister = () => useHasRole(GQL.RoleEnum.Admin);

export const CurrentUserProvider: React.FC<
  React.PropsWithChildren<ICurrentUserContext>
> = ({ user, children }) => {
  return (
    <CurrentUserContext.Provider value={{ user }}>
      {children}
    </CurrentUserContext.Provider>
  );
};
