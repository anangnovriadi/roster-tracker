import { create } from "@bufbuild/protobuf";
import { FieldMaskSchema } from "@bufbuild/protobuf/wkt";
import { Code, ConnectError } from "@connectrpc/connect";
import { useEffect, useState } from "react";
import { toast } from "react-hot-toast";
import { Button } from "@/components/ui/button";
import { Dialog, DialogContent, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { userServiceClient } from "@/connect";
import useLoading from "@/hooks/useLoading";
import { handleError } from "@/lib/error";
import { type User, User_Role, UserSchema } from "@/types/proto/api/user_service_pb";
import { useTranslate } from "@/utils/i18n";

interface Props {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  user?: User;
  onSuccess?: () => void;
}

type Field = "displayName" | "email" | "password";
type FieldErrors = Partial<Record<Field, string>>;

/** The server's limit for a username, which must start and end with a letter or digit. */
const MAX_USERNAME_LENGTH = 36;
const SUFFIX_LENGTH = 4;
const EMAIL_PATTERN = /^[^\s@]+@[^\s@]+\.[^\s@]+$/;

const randomSuffix = () => {
  const alphabet = "abcdefghijklmnopqrstuvwxyz0123456789";
  const values = crypto.getRandomValues(new Uint32Array(SUFFIX_LENGTH));
  return Array.from(values, (value) => alphabet[value % alphabet.length]).join("");
};

/** A username derived from a full name: "Siti Nur'aini" → "siti-nuraini-x7k2". */
export const buildUsername = (fullName: string, suffix: string): string => {
  const slug = fullName
    .normalize("NFD")
    .replace(/\p{Diacritic}/gu, "")
    .toLowerCase()
    .replace(/['’]/g, "")
    .replace(/[^a-z0-9]+/g, "-")
    .replace(/^-+|-+$/g, "")
    .slice(0, MAX_USERNAME_LENGTH - SUFFIX_LENGTH - 1)
    .replace(/-+$/g, "");
  return `${slug || "user"}-${suffix}`;
};

const isAlreadyExists = (error: unknown, field: "email" | "username") =>
  error instanceof ConnectError && error.code === Code.AlreadyExists && error.rawMessage.includes(field);

function CreateUserDialog({ open, onOpenChange, user: initialUser, onSuccess }: Props) {
  const t = useTranslate();
  const isCreating = !initialUser;
  const [displayName, setDisplayName] = useState("");
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [suffix, setSuffix] = useState(randomSuffix);
  const [errors, setErrors] = useState<FieldErrors>({});
  const requestState = useLoading(false);

  // Each opening starts from the edited user, or from an empty member.
  useEffect(() => {
    if (!open) return;
    setDisplayName(initialUser?.displayName ?? "");
    setEmail(initialUser?.email ?? "");
    setPassword("");
    setSuffix(randomSuffix());
    setErrors({});
  }, [open, initialUser]);

  const username = initialUser ? initialUser.username : buildUsername(displayName, suffix);

  const validate = (): FieldErrors => {
    const next: FieldErrors = {};
    if (!displayName.trim()) next.displayName = t("setting.member.full-name-required");
    if (!email.trim()) next.email = t("setting.member.email-required");
    else if (!EMAIL_PATTERN.test(email.trim())) next.email = t("setting.member.email-invalid");
    if (isCreating && !password) next.password = t("setting.member.password-required");
    return next;
  };

  const clearError = (field: Field) => setErrors((previous) => ({ ...previous, [field]: undefined }));

  const createUser = async (attempt: number): Promise<void> => {
    const candidate = attempt === 0 ? username : buildUsername(displayName, randomSuffix());
    try {
      await userServiceClient.createUser({
        // Roster Tracker has one admin account, so every account created here is a member.
        user: create(UserSchema, {
          username: candidate,
          displayName: displayName.trim(),
          email: email.trim(),
          password,
          role: User_Role.USER,
        }),
      });
    } catch (error) {
      // A random suffix can collide with an existing username; try a fresh one.
      if (attempt < 2 && isAlreadyExists(error, "username")) return createUser(attempt + 1);
      throw error;
    }
  };

  const handleConfirm = async () => {
    const nextErrors = validate();
    setErrors(nextErrors);
    if (Object.values(nextErrors).some(Boolean)) return;

    try {
      requestState.setLoading();
      if (isCreating) {
        await createUser(0);
        toast.success(t("setting.member.create-success", { name: displayName.trim() }));
      } else {
        const paths: string[] = [];
        if (displayName.trim() !== initialUser.displayName) paths.push("display_name");
        if (email.trim() !== initialUser.email) paths.push("email");
        if (password) paths.push("password");
        await userServiceClient.updateUser({
          user: create(UserSchema, { name: initialUser.name, displayName: displayName.trim(), email: email.trim(), password }),
          updateMask: create(FieldMaskSchema, { paths }),
        });
        toast.success(t("setting.member.update-success", { name: displayName.trim() }));
      }
      requestState.setFinish();
      onSuccess?.();
      onOpenChange(false);
    } catch (error: unknown) {
      requestState.setError();
      if (isAlreadyExists(error, "email")) {
        setErrors((previous) => ({ ...previous, email: t("setting.member.email-taken") }));
        return;
      }
      handleError(error, toast.error, { context: isCreating ? "Create user" : "Update user" });
    }
  };

  const requiredMark = (
    // Inside the label's text span, so the label's gap does not push it away from the word.
    <span aria-hidden className="ms-0.5 text-destructive">
      *
    </span>
  );

  const fieldError = (field: Field) =>
    errors[field] && (
      <p id={`${field}-error`} className="text-xs text-destructive">
        {errors[field]}
      </p>
    );

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-md">
        <DialogHeader>
          <DialogTitle>{`${isCreating ? t("common.create") : t("common.edit")} ${t("common.user")}`}</DialogTitle>
        </DialogHeader>
        <form
          className="flex flex-col gap-4"
          noValidate
          onSubmit={(event) => {
            event.preventDefault();
            void handleConfirm();
          }}
        >
          <div className="grid gap-2">
            <Label htmlFor="displayName">
              <span>
                {t("setting.member.full-name")}
                {requiredMark}
              </span>
            </Label>
            <Input
              id="displayName"
              aria-required
              autoComplete="off"
              placeholder={t("setting.member.full-name")}
              value={displayName}
              aria-invalid={Boolean(errors.displayName)}
              aria-describedby={errors.displayName ? "displayName-error" : undefined}
              onChange={(event) => {
                setDisplayName(event.target.value);
                clearError("displayName");
              }}
            />
            {fieldError("displayName")}
          </div>
          <div className="grid gap-2">
            <Label htmlFor="email">
              <span>
                {t("common.email")}
                {requiredMark}
              </span>
            </Label>
            <Input
              id="email"
              aria-required
              type="email"
              autoComplete="off"
              placeholder="name@example.com"
              value={email}
              aria-invalid={Boolean(errors.email)}
              aria-describedby={errors.email ? "email-error" : undefined}
              onChange={(event) => {
                setEmail(event.target.value);
                clearError("email");
              }}
            />
            {fieldError("email")}
          </div>
          <div className="grid gap-2">
            <Label htmlFor="username">{t("common.username")}</Label>
            <Input id="username" value={username} readOnly className="bg-muted text-muted-foreground" />
            <p className="text-xs text-muted-foreground">
              {isCreating ? t("setting.member.username-generated") : t("setting.member.username-fixed")}
            </p>
          </div>
          <div className="grid gap-2">
            <Label htmlFor="password">
              <span>
                {t("common.password")}
                {isCreating && requiredMark}
              </span>
            </Label>
            <Input
              id="password"
              aria-required={isCreating}
              type="password"
              autoComplete="new-password"
              placeholder={isCreating ? t("common.password") : t("setting.member.password-keep")}
              value={password}
              aria-invalid={Boolean(errors.password)}
              aria-describedby={errors.password ? "password-error" : undefined}
              onChange={(event) => {
                setPassword(event.target.value);
                clearError("password");
              }}
            />
            {fieldError("password")}
          </div>
          {/* Lets Enter submit the form; the visible buttons live in the dialog footer. */}
          <button type="submit" className="hidden" aria-hidden tabIndex={-1} />
        </form>
        <DialogFooter>
          <Button variant="ghost" disabled={requestState.isLoading} onClick={() => onOpenChange(false)}>
            {t("common.cancel")}
          </Button>
          <Button disabled={requestState.isLoading} onClick={() => void handleConfirm()}>
            {t("common.confirm")}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

export default CreateUserDialog;
