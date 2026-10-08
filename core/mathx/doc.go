// Package mathx holds the engine math types.
//
// Matrices are column-major. 2D: +X right, +Y down, positive rotation is
// clockwise (D08). 3D: right-handed, +Y up, -Z forward; the 2D plane maps to
// 3D XY with y -> -y (D82, Mat4Lift).
//
// QuatEuler(x, y, z) is Rz·Ry·Rx: X is applied first, then Y, then Z
// (extrinsic XYZ). Quat.Euler inverts it with y in [-π/2, π/2]; at the poles
// z is reported as 0.
//
// AABB2.Intersects is strict (boxes that only touch do not intersect),
// AABB3.Intersects is inclusive. Physics must not rely on either implicitly.
//
// Mat4.Decompose folds a negative determinant into scale X, so a matrix built
// with a negative Y or Z scale decomposes to an equivalent but different
// rotation/scale pair. Transform3D stores rotation and scale directly; only
// reparent with preserved world transform goes through Decompose.
package mathx
