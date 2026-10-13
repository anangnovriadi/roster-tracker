import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";
import { useTranslate } from "@/utils/i18n";

const NotePinnedMark = () => {
  const t = useTranslate();

  return (
    <Tooltip>
      <TooltipTrigger
        delay={400}
        render={<span role="img" aria-label={t("note.pinned")} />}
        className="absolute -right-px -top-px size-5 cursor-default rounded-tr-lg bg-warning [clip-path:polygon(0_0,100%_0,100%_100%)] transition-[width,height] duration-160 ease-out pointer-fine:group-hover/note:size-6 motion-reduce:transition-none"
      />
      <TooltipContent side="left" sideOffset={6}>
        {t("note.pinned")}
      </TooltipContent>
    </Tooltip>
  );
};

export default NotePinnedMark;
