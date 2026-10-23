import { useContext } from "react";
import {
  TextInput,
  Password,
  TextArea,
  CheckboxControl,
  CheckboxGroup,
} from "./controls";
import {
  SkeletonRoot,
  SkeletonBlock,
  DescriptionsRoot,
  DescriptionItem,
  ListRoot,
  ListItem,
  Text,
  Paragraph,
  Title,
  TextLink,
} from "./display";
import { RadioControl, RadioGroup } from "./Radio";
import { FeedbackProvider } from "./overlays";
import { FeedbackContext } from "./feedbackContext";
import { useBreakpoint } from "./useBreakpoint";

export const Input = Object.assign(TextInput, { Password, TextArea });
export const Checkbox = Object.assign(CheckboxControl, {
  Group: CheckboxGroup,
});
export const Skeleton = Object.assign(SkeletonRoot, {
  Avatar: SkeletonBlock,
  Input: SkeletonBlock,
  Node: SkeletonBlock,
});
export const Descriptions = Object.assign(DescriptionsRoot, {
  Item: DescriptionItem,
});
export const List = Object.assign(ListRoot, { Item: ListItem });
export const Typography = { Text, Paragraph, Title, Link: TextLink };
export const Grid = { useBreakpoint };
export const Radio = Object.assign(RadioControl, {
  Button: RadioControl,
  Group: RadioGroup,
});
export const App = Object.assign(FeedbackProvider, {
  useApp: () => useContext(FeedbackContext),
});
