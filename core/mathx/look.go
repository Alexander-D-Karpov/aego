package mathx

func QuatYawPitch(yaw, pitch float32) Quat {
	return QuatAxisAngle(Vec3{Y: 1}, yaw).Mul(QuatAxisAngle(Vec3{X: 1}, pitch))
}

func YawPitchOf(dir Vec3) (yaw, pitch float32) {
	h := Sqrt(dir.X*dir.X + dir.Z*dir.Z)
	return Atan2(-dir.X, -dir.Z), Atan2(dir.Y, h)
}

func QuatLookDir(dir Vec3) Quat {
	if dir.LenSq() < 1e-12 {
		return QuatIdentity()
	}
	y, p := YawPitchOf(dir)
	return QuatYawPitch(y, p)
}
