import React from "react";
import { Route, Switch } from "react-router-dom";
import { ModifyRoute } from "src/components/Shared/ModifyRoute";
import { Helmet } from "react-helmet";
import { useTitleProps } from "src/hooks/title";
import Studio from "./StudioDetails/Studio";
import StudioCreate from "./StudioDetails/StudioCreate";
import { FilteredStudioList } from "./StudioList";
import { View } from "../List/views";

const Studios: React.FC = () => {
  return <FilteredStudioList view={View.Studios} />;
};

const StudioRoutes: React.FC = () => {
  const titleProps = useTitleProps({ id: "studios" });
  return (
    <>
      <Helmet {...titleProps} />
      <Switch>
        <Route exact path="/studios" component={Studios} />
        <ModifyRoute exact path="/studios/new" component={StudioCreate} />
        <Route path="/studios/:id/:tab?" component={Studio} />
      </Switch>
    </>
  );
};

export default StudioRoutes;
