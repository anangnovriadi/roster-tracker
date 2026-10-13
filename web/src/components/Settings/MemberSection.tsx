import { sortBy } from "lodash-es";
import { MoreVerticalIcon, PlusIcon } from "lucide-react";
import { useMemo, useState } from "react";
import toast from "react-hot-toast";
import ConfirmDialog from "@/components/ConfirmDialog";
import { Button } from "@/components/ui/button";
import useCurrentUser from "@/hooks/useCurrentUser";
import { useDialog } from "@/hooks/useDialog";
import { useDeleteUser, useListUsers } from "@/hooks/useUserQueries";
import { handleError } from "@/lib/error";
import { User, User_Role } from "@/types/proto/api/user_service_pb";
import { useTranslate } from "@/utils/i18n";
import CreateUserDialog from "../CreateUserDialog";
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuTrigger } from "../ui/dropdown-menu";
import SettingSection from "./SettingSection";
import SettingTable from "./SettingTable";

const MemberSection = () => {
  const t = useTranslate();
  const currentUser = useCurrentUser();
  const { data: users = [], refetch: refetchUsers } = useListUsers();
  const deleteUserMutation = useDeleteUser();
  const createDialog = useDialog();
  const editDialog = useDialog();
  const [editingUser, setEditingUser] = useState<User | undefined>();
  // The one admin account leads the list; members follow in the order they joined.
  const sortedUsers = useMemo(() => sortBy(users, [(user) => (user.role === User_Role.ADMIN ? 0 : 1), "id"]), [users]);
  const [deleteTarget, setDeleteTarget] = useState<User | undefined>(undefined);

  const stringifyUserRole = (role: User_Role) => (role === User_Role.ADMIN ? t("setting.member.admin") : t("setting.member.user"));

  const handleCreateUser = () => {
    setEditingUser(undefined);
    createDialog.open();
  };

  const handleEditUser = (user: User) => {
    setEditingUser(user);
    editDialog.open();
  };

  const handleDeleteUserClick = (user: User) => {
    setDeleteTarget(user);
  };

  const confirmDeleteUser = () => {
    if (!deleteTarget) return;
    const { username, name } = deleteTarget;
    deleteUserMutation.mutate(name, {
      onSuccess: () => {
        setDeleteTarget(undefined);
        toast.success(t("setting.member.delete-success", { username }));
      },
      onError: (error) => {
        setDeleteTarget(undefined);
        handleError(error, toast.error, { context: "Delete user" });
      },
    });
  };

  return (
    <SettingSection
      title={t("setting.member.list-title")}
      actions={
        <Button onClick={handleCreateUser}>
          <PlusIcon className="size-4" />
          {t("setting.member.create-member")}
        </Button>
      }
    >
      <SettingTable
        columns={[
          {
            key: "number",
            header: t("setting.member.number-column"),
            className: "w-px tabular-nums",
            render: (_, user: User) => sortedUsers.indexOf(user) + 1,
          },
          {
            key: "name",
            header: t("setting.member.full-name"),
            render: (_, user: User) => (
              <span className="text-foreground">
                {user.displayName || "—"}
                {currentUser?.name === user.name ? (
                  <span className="ms-2 text-xs text-muted-foreground">{t("common.yourself")}</span>
                ) : null}
              </span>
            ),
          },
          {
            key: "username",
            header: t("common.username"),
            render: (_, user: User) => `@${user.username}`,
          },
          {
            key: "email",
            header: t("common.email"),
            render: (_, user: User) => user.email || "—",
          },
          {
            key: "role",
            header: t("setting.member.role-column"),
            render: (_, user: User) => stringifyUserRole(user.role),
          },
          {
            key: "actions",
            header: "",
            className: "w-px text-right",
            render: (_, user: User) =>
              currentUser?.name === user.name ? null : (
                <DropdownMenu>
                  <DropdownMenuTrigger render={<Button variant="outline" size="sm" />}>
                    <MoreVerticalIcon className="w-4 h-auto" />
                  </DropdownMenuTrigger>
                  <DropdownMenuContent align="end" sideOffset={2}>
                    <DropdownMenuItem onClick={() => handleEditUser(user)}>{t("common.update")}</DropdownMenuItem>
                    <DropdownMenuItem onClick={() => handleDeleteUserClick(user)} className="text-destructive focus:text-destructive">
                      {t("setting.member.delete-member")}
                    </DropdownMenuItem>
                  </DropdownMenuContent>
                </DropdownMenu>
              ),
          },
        ]}
        data={sortedUsers}
        emptyMessage={t("setting.member.no-members-found")}
        getRowKey={(user) => user.name}
      />

      {/* Create User Dialog */}
      <CreateUserDialog open={createDialog.isOpen} onOpenChange={createDialog.setOpen} onSuccess={refetchUsers} />

      {/* Edit User Dialog */}
      <CreateUserDialog open={editDialog.isOpen} onOpenChange={editDialog.setOpen} user={editingUser} onSuccess={refetchUsers} />

      <ConfirmDialog
        open={!!deleteTarget}
        onOpenChange={(open) => !open && setDeleteTarget(undefined)}
        title={deleteTarget ? t("setting.member.delete-warning", { username: deleteTarget.username }) : ""}
        description={deleteTarget ? t("setting.member.delete-warning-description") : ""}
        confirmLabel={t("common.delete")}
        cancelLabel={t("common.cancel")}
        onConfirm={confirmDeleteUser}
        confirmVariant="destructive"
      />
    </SettingSection>
  );
};

export default MemberSection;
