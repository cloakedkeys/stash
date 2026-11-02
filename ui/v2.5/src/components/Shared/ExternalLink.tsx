import TextUtils from "src/utils/text";

type IExternalLinkProps = JSX.IntrinsicElements["a"];

export const ExternalLink: React.FC<IExternalLinkProps> = (props) => {
  // a link that would run code is shown as plain text. These can come from a
  // scraper as well as from typing.
  if (props.href && TextUtils.isScriptURL(props.href)) {
    return <span className={props.className}>{props.children}</span>;
  }

  return <a target="_blank" rel="noopener noreferrer" {...props} />;
};
