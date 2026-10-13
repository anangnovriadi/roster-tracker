import { Dialog, DialogContent, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { useTranslate } from "@/utils/i18n";

/** Who makes Roster Tracker, with the copyright line. */
const AboutDialog = ({ open, onOpenChange }: { open: boolean; onOpenChange: (open: boolean) => void }) => {
  const t = useTranslate();
  const year = new Date().getFullYear();

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-sm">
        <DialogHeader className="items-center text-center">
          <img src="/logo.svg" alt="" className="mb-2 size-14" />
          <DialogTitle>Roster Tracker</DialogTitle>
        </DialogHeader>
        <div className="flex flex-col items-center gap-3 text-center">
          <p className="text-xs text-muted-foreground">{t("about.copyright", { year, company: "Cloud Creatures" })}</p>
        </div>
      </DialogContent>
    </Dialog>
  );
};

export default AboutDialog;
