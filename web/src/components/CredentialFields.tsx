import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { useTranslate } from "@/utils/i18n";

interface Props {
  idPrefix: string;
  username: string;
  password: string;
  passwordAutoComplete: "current-password" | "new-password";
  /** Sign-in also accepts an email address in the username field. */
  acceptEmail?: boolean;
  readOnly?: boolean;
  onUsernameChange: (username: string) => void;
  onPasswordChange: (password: string) => void;
}

// Username + password field pair shared by the sign-in and sign-up forms.
const CredentialFields = ({
  idPrefix,
  username,
  password,
  passwordAutoComplete,
  acceptEmail,
  readOnly,
  onUsernameChange,
  onPasswordChange,
}: Props) => {
  const t = useTranslate();
  const usernameLabel = acceptEmail ? t("common.username-or-email") : t("common.username");

  return (
    <>
      <div className="flex flex-col gap-1.5">
        <Label htmlFor={`${idPrefix}-username`}>{usernameLabel}</Label>
        <Input
          id={`${idPrefix}-username`}
          type="text"
          readOnly={readOnly}
          placeholder={usernameLabel}
          value={username}
          autoComplete="username"
          autoCapitalize="off"
          spellCheck={false}
          onChange={(e) => onUsernameChange(e.target.value)}
          required
        />
      </div>
      <div className="flex flex-col gap-1.5">
        <Label htmlFor={`${idPrefix}-password`}>{t("common.password")}</Label>
        <Input
          id={`${idPrefix}-password`}
          type="password"
          readOnly={readOnly}
          placeholder={t("common.password")}
          value={password}
          autoComplete={passwordAutoComplete}
          autoCapitalize="off"
          spellCheck={false}
          onChange={(e) => onPasswordChange(e.target.value)}
          required
        />
      </div>
    </>
  );
};

export default CredentialFields;
