import { Link } from "react-router-dom";
import { FormattedMessage } from "react-intl";
import NavUtils from "src/utils/navigation";

export const GroupPartLabel = ({
  group,
  sceneNumber,
}: {
  group: { id: string; name: string; scene_count: number };
  sceneNumber: number;
}) => (
  <Link to={NavUtils.makeGroupScenesUrl(group)}>
    <FormattedMessage
      id={
        sceneNumber >= 1 && sceneNumber <= group.scene_count
          ? "scene_part_of"
          : "scene_part"
      }
      values={{ number: sceneNumber, count: group.scene_count }}
    />
  </Link>
);
