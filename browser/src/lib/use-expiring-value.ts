import { useEffect, useState } from 'react';

/** Feedback expires by wall-clock time, including when background timers pause. */
export function useExpiringValue<T>(empty: T, duration: number) {
  const [value, setValue] = useState(empty);
  useEffect(() => {
    if (Object.is(value, empty)) return;
    const deadline = Date.now() + duration;
    let timer: ReturnType<typeof setTimeout>;
    const check = () => {
      clearTimeout(timer);
      const remaining = deadline - Date.now();
      if (remaining <= 0) setValue(current => Object.is(current, value) ? empty : current);
      else timer = setTimeout(check, remaining);
    };
    window.addEventListener('focus', check);
    document.addEventListener('visibilitychange', check);
    check();
    return () => {
      clearTimeout(timer);
      window.removeEventListener('focus', check);
      document.removeEventListener('visibilitychange', check);
    };
  }, [value, empty, duration]);
  return [value, setValue] as const;
}
