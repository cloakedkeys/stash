import React from "react";
import { Route, Switch } from "react-router-dom";
import { ModifyRoute } from "src/components/Shared/ModifyRoute";
import { Helmet } from "react-helmet";
import { useTitleProps } from "src/hooks/title";
import Tag from "./TagDetails/Tag";
import TagCreate from "./TagDetails/TagCreate";
import { FilteredTagList } from "./TagList";

const Tags: React.FC = () => {
  return <FilteredTagList />;
};

const TagRoutes: React.FC = () => {
  const titleProps = useTitleProps({ id: "tags" });
  return (
    <>
      <Helmet {...titleProps} />
      <Switch>
        <Route exact path="/tags" component={Tags} />
        <ModifyRoute exact path="/tags/new" component={TagCreate} />
        <Route path="/tags/:id/:tab?" component={Tag} />
      </Switch>
    </>
  );
};

export default TagRoutes;
