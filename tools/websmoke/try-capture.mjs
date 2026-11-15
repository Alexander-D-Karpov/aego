const fields = ["x", "y", "width", "height", "canvasWidth", "canvasHeight"];

export function sameCaptureViewport(a, b) {
    return !!a && !!b && fields.every(key => Number.isFinite(a[key]) && a[key] === b[key]);
}

export function neutralCapturePoint(viewport, bounds) {
    if (!viewport || !fields.every(key => Number.isFinite(viewport[key])) ||
        viewport.width <= 0 || viewport.height <= 0 || viewport.canvasWidth < 4 || viewport.canvasHeight < 4 ||
        viewport.x < 0 || viewport.y < 0 || viewport.x + viewport.width > viewport.canvasWidth ||
        viewport.y + viewport.height > viewport.canvasHeight || !bounds ||
        ![bounds.x, bounds.y, bounds.width, bounds.height].every(Number.isFinite) || bounds.width < 4 || bounds.height < 4) {
        throw new Error("invalid Game View or canvas capture bounds");
    }
    const {canvasWidth: w, canvasHeight: h} = viewport;
    // The surrounding Try stage has rounded, clipped corners; measure this
    // inset in CSS pixels rather than in the runtime's logical coordinates.
    const mx = Math.min(12 * w / bounds.width, w / 4), my = Math.min(12 * h / bounds.height, h / 4);
    const corners = [[w - mx, my], [mx, my], [w - mx, h - my], [mx, h - my]];
    const outside = ([x, y]) => x < viewport.x || y < viewport.y ||
        x >= viewport.x + viewport.width || y >= viewport.y + viewport.height;
    const point = corners.find(outside);
    if (!point) throw new Error("no neutral pointer location outside Game View");
    return {x: bounds.x + point[0] * bounds.width / w, y: bounds.y + point[1] * bounds.height / h};
}

// Use a real pointer move inside the canvas, away from the captured Game View.
export async function parkCapturePointer(page, viewport) {
    const canvas = page.locator(".try canvas");
    await canvas.scrollIntoViewIfNeeded();
    const point = neutralCapturePoint(viewport, await canvas.boundingBox());
    const onCanvas = await page.evaluate(({x, y}) => document.elementFromPoint(x, y) === document.querySelector(".try canvas"), point);
    if (!onCanvas) throw new Error("neutral capture location is not on the canvas");
    await page.mouse.move(point.x, point.y);
}
