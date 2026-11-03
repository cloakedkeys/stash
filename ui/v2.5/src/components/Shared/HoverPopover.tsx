import React, { useState, useCallback, useEffect, useRef } from "react";
import { Overlay, Popover, OverlayProps } from "react-bootstrap";
import { PatchComponent } from "src/patch";
import { Icon } from "./Icon";
import { viewportModifiers } from "./viewportPopper";
import { faExclamationTriangle } from "@fortawesome/free-solid-svg-icons";

// A phone can't hover, so there a tap opens and closes the pop-up instead,
// and a tap elsewhere closes it (#1972). Checked on each event, as a device
// can switch between mouse and touch.
function canHover() {
  return !window.matchMedia?.("(hover: none)").matches;
}

interface IHoverPopover {
  enterDelay?: number;
  leaveDelay?: number;
  content: JSX.Element[] | JSX.Element | string;
  className?: string;
  placement?: OverlayProps["placement"];
  popperConfig?: OverlayProps["popperConfig"];
  onOpen?: () => void;
  onClose?: () => void;
  target?: React.RefObject<HTMLElement>;
  children?: React.ReactNode;
}

export const HoverPopover: React.FC<IHoverPopover> = PatchComponent(
  "HoverPopover",
  ({
    enterDelay = 200,
    leaveDelay = 200,
    content,
    children,
    className,
    placement = "top",
    popperConfig,
    onOpen,
    onClose,
    target,
  }) => {
    const [show, setShow] = useState(false);
    const [triggerElement, setTriggerElement] = useState<HTMLDivElement | null>(
      null
    );
    const enterTimer = useRef<number>();
    const leaveTimer = useRef<number>();
    const popoverId = useRef(
      `hover-popover-${Math.random().toString(36).slice(2)}`
    );

    const setTriggerRef = useCallback((element: HTMLDivElement | null) => {
      setTriggerElement(element);
    }, []);

    const getOverlayTarget = useCallback(() => {
      return target?.current ?? triggerElement;
    }, [target, triggerElement]);

    const handleMouseEnter = useCallback(() => {
      if (!canHover()) return;
      window.clearTimeout(leaveTimer.current);
      enterTimer.current = window.setTimeout(() => {
        setShow(true);
        onOpen?.();
      }, enterDelay);
    }, [enterDelay, onOpen]);

    const handleMouseLeave = useCallback(() => {
      if (!canHover()) return;
      window.clearTimeout(enterTimer.current);
      leaveTimer.current = window.setTimeout(() => {
        setShow(false);
        onClose?.();
      }, leaveDelay);
    }, [leaveDelay, onClose]);

    const setShown = useCallback(
      (shown: boolean) => {
        setShow(shown);
        if (shown) onOpen?.();
        else onClose?.();
      },
      [onOpen, onClose]
    );

    const handleTap = useCallback(() => {
      if (canHover()) return;
      setShown(!show);
    }, [show, setShown]);

    const handleHide = useCallback(() => {
      if (canHover()) return;
      setShown(false);
    }, [setShown]);

    useEffect(
      () => () => {
        window.clearTimeout(enterTimer.current);
        window.clearTimeout(leaveTimer.current);
      },
      []
    );

    return (
      <>
        <div
          className={className}
          onMouseEnter={handleMouseEnter}
          onMouseLeave={handleMouseLeave}
          onClick={handleTap}
          ref={setTriggerRef}
        >
          {children}
        </div>
        {getOverlayTarget() && (
          <Overlay
            show={show}
            placement={placement}
            target={getOverlayTarget}
            container={document.body}
            flip
            rootClose
            onHide={handleHide}
            popperConfig={{
              ...popperConfig,
              modifiers: [
                ...viewportModifiers,
                ...(popperConfig?.modifiers ?? []),
              ],
            }}
          >
            <Popover
              onMouseEnter={handleMouseEnter}
              onMouseLeave={handleMouseLeave}
              id={popoverId.current}
              className="hover-popover-content"
            >
              {content}
            </Popover>
          </Overlay>
        )}
      </>
    );
  }
);

// convenience component to set the padding on popover content
export const PopoverCard: React.FC<
  React.PropsWithChildren<{ className?: string }>
> = ({ className, children }) => {
  return <div className={`popover-card ${className}`}>{children}</div>;
};

export const WarningHoverPopover: React.FC<IHoverPopover> = PatchComponent(
  "WarningHoverPopover",
  ({ children, ...props }) => (
    <HoverPopover {...props} className="warning-hover-popover">
      <Icon icon={faExclamationTriangle} />
    </HoverPopover>
  )
);
