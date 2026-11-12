import React from "react";
import { Button, Dropdown } from "react-bootstrap";
import { useIntl } from "react-intl";
import { faThumbtack } from "@fortawesome/free-solid-svg-icons";
import { Icon } from "./Icon";

export const ScraperMenuItem: React.FC<{
  scraper: { id: string; name: string };
  pinned: boolean;
  saving: boolean;
  togglePin: (id: string) => void;
  onSelect: () => void;
}> = ({ scraper, pinned, saving, togglePin, onSelect }) => {
  const intl = useIntl();
  const label = intl.formatMessage({
    id: pinned ? "actions.unpin_scraper" : "actions.pin_scraper",
  });
  return (
    <div className="d-flex align-items-center">
      <Dropdown.Item onClick={onSelect}>{scraper.name}</Dropdown.Item>
      <Button
        variant="minimal"
        title={label}
        aria-label={`${label}: ${scraper.name}`}
        aria-pressed={pinned}
        disabled={saving}
        onClick={(e) => {
          e.preventDefault();
          e.stopPropagation();
          togglePin(scraper.id);
        }}
      >
        <Icon icon={faThumbtack} className={pinned ? "" : "tilted"} />
      </Button>
    </div>
  );
};
