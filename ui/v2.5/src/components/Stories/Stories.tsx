import React from "react";
import { Route, Switch } from "react-router-dom";
import { Helmet } from "react-helmet";
import { useTitleProps } from "src/hooks/title";
import { lazyComponent } from "src/utils/lazyComponent";
import { View } from "../List/views";

const StoryList = lazyComponent(() => import("./StoryList"));
const Story = lazyComponent(() => import("./StoryDetails/Story"));

const Stories: React.FC = () => {
  return <StoryList view={View.Stories} />;
};

const StoryRoutes: React.FC = () => {
  const titleProps = useTitleProps({ id: "stories" });
  return (
    <>
      <Helmet {...titleProps} />
      <Switch>
        <Route exact path="/stories" component={Stories} />
        <Route path="/stories/:id" component={Story} />
      </Switch>
    </>
  );
};

export default StoryRoutes;
