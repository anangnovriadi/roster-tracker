import { Globe2Icon, LockIcon, type LucideIcon, UserLockIcon, UsersIcon } from "lucide-react";
import { Visibility } from "@/types/proto/api/note_service_pb";

export interface VisibilityOption {
  value: Visibility;
  /** Proto enum name, as stored in user settings and sent in filter expressions. */
  name: "PRIVATE" | "SPACE" | "PROTECTED" | "PUBLIC";
  labelKey: "note.visibility.private" | "note.visibility.space" | "note.visibility.protected" | "note.visibility.public";
  descriptionKey:
    | "note.visibility.private-description"
    | "note.visibility.space-description"
    | "note.visibility.protected-description"
    | "note.visibility.public-description";
  icon: LucideIcon;
  /** SPACE only means anything inside a Space, so it is offered contextually rather than as a standing choice. */
  requiresSpace: boolean;
}

/**
 * The single source of truth for how each audience is named, described and drawn.
 * Ordered by widening audience. Every visibility surface derives from this, so a new
 * enum value is added here once instead of in each picker, icon map and converter.
 */
export const VISIBILITY_OPTIONS: readonly VisibilityOption[] = [
  {
    value: Visibility.PRIVATE,
    name: "PRIVATE",
    labelKey: "note.visibility.private",
    descriptionKey: "note.visibility.private-description",
    icon: LockIcon,
    requiresSpace: false,
  },
  {
    value: Visibility.SPACE,
    name: "SPACE",
    labelKey: "note.visibility.space",
    descriptionKey: "note.visibility.space-description",
    icon: UserLockIcon,
    requiresSpace: true,
  },
  {
    value: Visibility.PROTECTED,
    name: "PROTECTED",
    labelKey: "note.visibility.protected",
    descriptionKey: "note.visibility.protected-description",
    icon: UsersIcon,
    requiresSpace: false,
  },
  {
    value: Visibility.PUBLIC,
    name: "PUBLIC",
    labelKey: "note.visibility.public",
    descriptionKey: "note.visibility.public-description",
    icon: Globe2Icon,
    requiresSpace: false,
  },
];

export const getVisibilityOption = (visibility: Visibility): VisibilityOption | undefined =>
  VISIBILITY_OPTIONS.find((option) => option.value === visibility);

/**
 * Audiences a note can be set to. SPACE is offered when the note is placed in a
 * Space, and also when a legacy/inconsistent note already uses it so the control
 * can still name its current audience instead of silently downgrading it.
 */
export const getAssignableVisibilityOptions = (options: { hasSpacePlacement: boolean; current?: Visibility }): VisibilityOption[] =>
  VISIBILITY_OPTIONS.filter((option) => !option.requiresSpace || options.hasSpacePlacement || option.value === options.current);

/** Audiences offered as a persistent default. A Space-scoped default has no meaning outside a Space. */
export const DEFAULT_VISIBILITY_OPTIONS: readonly VisibilityOption[] = VISIBILITY_OPTIONS.filter((option) => !option.requiresSpace);

/** Unknown or empty names fall back to PRIVATE so a bad value never widens a note's audience. */
export const convertVisibilityFromString = (visibility: string) =>
  VISIBILITY_OPTIONS.find((option) => option.name === visibility)?.value ?? Visibility.PRIVATE;

export const convertVisibilityToString = (visibility: Visibility) => getVisibilityOption(visibility)?.name ?? "PRIVATE";
