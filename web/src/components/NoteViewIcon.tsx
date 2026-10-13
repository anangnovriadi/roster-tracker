import { ParenthesesIcon } from "lucide-react";
import CustomIcon from "@/components/CustomIcon";
import type { NoteView_Icon } from "@/types/proto/api/user_service_pb";

const NoteViewIcon = ({ icon, className }: { icon?: NoteView_Icon; className?: string }) => (
  <CustomIcon icon={icon} className={className} fallback={ParenthesesIcon} />
);

export default NoteViewIcon;
