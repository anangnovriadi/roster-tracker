import { LoaderCircleIcon } from "lucide-react";
import { useDelayedFlag } from "@/hooks/useDelayedFlag";
import { LOADING_INDICATOR_DELAY_MS } from "@/lib/constants";
import { cn } from "@/lib/utils";

/** A centered spinner for a page that is still loading; it waits a moment so fast loads never flash it. */
const PageLoader = ({ className }: { className?: string }) => {
  const visible = useDelayedFlag(true, LOADING_INDICATOR_DELAY_MS);
  return (
    <div role="status" aria-label="Loading" className={cn("flex w-full flex-1 items-center justify-center", className)}>
      {visible && <LoaderCircleIcon aria-hidden className="size-8 animate-spin text-muted-foreground" />}
    </div>
  );
};

export default PageLoader;
